package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/nstehr/vimy/currie/baml_client"
	"github.com/nstehr/vimy/currie/baml_client/types"
)

// Insight is what the model made of a replay.
//
// Mirrored rather than aliased to the generated type, so the templates don't
// depend on generated code and a build with no model still compiles the page.
type Insight struct {
	Summary    string
	Findings   []Finding
	Suggestion string
	Caveat     string
	// Nil when the blocking was not doctrinal — rewriting the directive would not
	// help, and suggesting it anyway is confidently useless.
	Directive *DirectiveAdvice
}

// DirectiveAdvice is how to change the sentence that produced the weights.
type DirectiveAdvice struct {
	Diagnosis  string
	Revision   string
	Confidence float64
}

type Finding struct {
	Claim      string
	Evidence   string
	Confidence float64
}

// llmInsighter reads a replay through BAML.
type llmInsighter struct{}

// newInsighter returns nil with no key configured. Not an error: the report is
// the product, the prose is commentary.
func newInsighter() (insighter, error) {
	if os.Getenv("OPENAI_API_KEY") == "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		return nil, nil
	}
	return &llmInsighter{}, nil
}

func (l *llmInsighter) Read(ctx context.Context, r *Replay) (*Insight, error) {
	out, err := baml_client.ReadPostMortem(ctx, facts(r))
	if err != nil {
		return nil, err
	}
	ins := &Insight{Summary: out.Summary, Suggestion: out.Suggestion, Caveat: out.Caveat}
	if a := out.Directive_advice; a != nil {
		ins.Directive = &DirectiveAdvice{
			Diagnosis: a.Diagnosis, Revision: a.Revision, Confidence: a.Confidence,
		}
	}
	for _, f := range out.Findings {
		ins.Findings = append(ins.Findings, Finding{
			Claim: f.Claim, Evidence: f.Evidence, Confidence: f.Confidence,
		})
	}
	return ins, nil
}

// facts projects a replay into what the model reads — the same numbers the page
// shows and only those, so it cannot make a claim the reader can't check.
func facts(r *Replay) types.GameFacts {
	v := buildWith("", r.Report, r.Windows_, r.Firings, r.Game.DurationTicks, r.Game.OurFaction, r.Doctrines)

	f := types.GameFacts{
		Our_faction:      r.Game.OurFaction,
		Opponent_faction: r.Game.OpponentFaction,
		Outcome:          outcome(r.Game.Won),
		Duration_ticks:   int64(r.Game.DurationTicks),
		Doctrine_windows: int64(r.Windows),
		States_replayed:  int64(r.Report.States),
		Rules_in_set:     int64(len(r.Report.Rules)),
		Never_fired:      int64(v.NeverFired),
		No_single_cause:  int64(v.NoCulprit),
		Doctrine_names:   r.DoctrineNames,
		Faction:          r.Game.OurFaction,
		Directive:        r.Game.Directive,
	}

	// Sampled: a long game writes forty doctrines with long rationales, and the
	// model needs the pattern rather than every instance of it.
	step := max(len(r.Doctrines)/8, 1)
	for i := 0; i < len(r.Doctrines); i += step {
		d := r.Doctrines[i]
		f.Doctrines = append(f.Doctrines, types.DoctrineChoice{
			Name: d.Name, Rationale: d.Rationale,
			Infantry_weight: d.InfantryWeight, Vehicle_weight: d.VehicleWeight,
			Air_weight: d.AirWeight, Tech_priority: d.TechPriority,
			Economy_priority: d.EconomyPriority, Aggression: d.Aggression,
		})
	}
	for _, s := range v.Sites {
		// Guards, lines whose rules already work, and clauses this faction can
		// never satisfy are not findings. Ranking puts them last, but the model
		// should not see them at all — it has led with one before.
		if s.Live() || s.Guard() || s.Impossible != "" || s.QuietAxis != "" {
			continue
		}
		f.Top_clauses = append(f.Top_clauses, types.BlockedClause{
			Working:   int64(s.Working),
			Source:    s.Source,
			File:      s.File,
			Line:      int64(s.Line),
			Sole:      int64(s.Sole),
			Sole_rate: s.SoleRate,
			Blocked:   int64(s.Blocked),
			Rules:     s.Rules,
		})
	}
	dead := append([]deadRule(nil), v.Dead...)
	sort.SliceStable(dead, func(i, j int) bool {
		return dead[i].Culprit.Sole > dead[j].Culprit.Sole
	})
	if len(dead) > 12 {
		dead = dead[:12]
	}
	for _, s := range v.Sensitivity {
		f.Sensitivity = append(f.Sensitivity, types.ClauseSensitivity{
			Source: s.Source, File: filepath.Base(s.File), Line: int64(s.Line),
			Rules: int64(s.Rules), Windows: int64(s.Windows),
			Low_rate: s.Low, High_rate: s.High,
			Doctrinal: s.Doctrinal, Knob: s.Knob,
			Knob_low: s.KnobLow, Knob_high: s.KnobHigh,
			Rate_low: s.RateLow, Rate_high: s.RateHigh,
			Verdict: s.Verdict(),
		})
	}
	for _, g := range v.Gates {
		f.Gates = append(f.Gates, types.TunableGate{
			Source: g.Source, File: g.File, Line: int64(g.Line), Rules: int64(len(g.Rules)),
			At: g.AtHigh, Blocked: int64(g.Blocked), Relief: int64(g.Relief),
			Relief_at: g.ReliefHigh, Reached: g.Reached,
		})
	}
	for _, c := range v.Chains {
		f.Missing = append(f.Missing, types.MissingThing{
			Rule: c.Rule, Clause: c.Clause, Thing: c.Thing, Maker: c.Maker,
			Maker_matched: int64(c.MakerMatched), Maker_culprit: c.MakerCulprit,
			Verdict: c.Verdict(),
		})
	}
	for _, d := range dead {
		f.Dead_rules = append(f.Dead_rules, types.DeadRule{
			Name:         d.Name,
			Seen:         int64(d.Seen),
			Culprit:      fmt.Sprintf("%s (%s:%d)", d.Culprit.Source, filepath.Base(d.Culprit.File), d.Culprit.Line),
			Culprit_sole: int64(d.Culprit.Sole),
		})
	}
	return f
}
