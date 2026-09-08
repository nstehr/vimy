package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nstehr/vimy/vimy-core/store"
)

// A source line, and every rule it stops.
//
// The most useful axis in the whole report: a `require` written once inside a
// def — `reserves(cost)`, `affordable(cost)` — is inlined into dozens of rules,
// so one line can be the single thing standing between the AI and a third of
// its behaviour. Per-rule counts never show that; this does.
type site struct {
	File    string
	Line    int
	Source  string
	Sole    int
	Blocked int
	Rules   []string
	SolePct float64
	// Sole as a share of the states replayed. The count alone is not comparable
	// across games — a game a third shorter yields a third fewer of everything,
	// and reading two raw counts side by side reports the length difference as
	// a finding.
	SoleRate float64
	// What this line asks for that the faction can never have, empty when the
	// clause is satisfiable in principle. Such a line is correct and inert, and
	// ranking it as a top blocker is noise.
	Impossible string
	// How many of the rules this line blocks already fire and do something in
	// the real game. A line whose rules all already work is not where a problem
	// is, however often the sample saw it block — the sample simply missed the
	// moments they fired.
	Working int
}

// Live reports whether every rule this line blocks already works.
func (s site) Live() bool { return s.Working > 0 && s.Working == len(s.Rules) }

type clauseBar struct {
	clauseReport
	SolePct    float64
	OtherPct   float64
	IsCulprit  bool
	ShortFile  string
	NeverFired bool
}

type deadRule struct {
	Name     string
	Category string
	// What the rule did in the real game. `Acted` is -1 when unmeasured.
	Matched, Acted int
	Seen           int
	Blocked        int
	Preempted      int
	Culprit        *clauseReport
	Clauses        []clauseBar
	SolePct        float64
}

// LateStart is a rule that worked, but not until late.
//
// The replay can say what blocked a rule; it cannot say when a rule that
// worked did its work, because a blame count has no clock. That gap made a
// scouting rule which fired seven times look identical to one that was broken,
// when the finding was that nothing scouted until the game was half over.
type LateStart struct {
	Name     string
	Category string
	Matched  int
	Acted    int
	// Ticks, and how far into the game that is.
	FirstTick, LastTick int
	Pct                 float64
	// Whether onset counts as late. Reactive rules are legitimately late, so
	// this marks rather than filters.
	Late bool
	// Span between first and last action, as a share of the game. A rule that
	// acts once is a different thing from one that acts throughout.
	SpanPct float64
}

// finding is one fact worth reading first. The report grew seven headline
// numbers and nine sections, all weighted equally, and the reader (me, all day)
// had to reconstruct the same three questions each time: what blocked the most,
// what fired and achieved nothing, and what arrived too late. Those are
// computed here so the page can lead with them.
//
// Facts, not conclusions. "build-war-factory first acted at 57%" is something
// the data says; "the war factory was too late" is a judgement the reader makes.
type finding struct {
	Headline string
	Detail   string
}

type view struct {
	Title      string
	States     int
	RuleCount  int
	NeverFired int
	NoCulprit  int
	// Rules the replay never caught firing but which the engine recorded acting.
	SampleMissed int
	// Rules that worked, but did not start until late.
	Late []LateStart
	// How far into the game the game's own duration was, for the axis.
	DurationTicks int
	// Who we played, so a clause can say which faction can never satisfy it.
	Faction string
	// The two or three facts worth reading before anything else.
	Findings []finding
	// Working rules whose onset the timeline did not have room for. Counted,
	// because a silent cap reads as "this is all of them".
	LateOmitted int
	// Blame sites ranked below the cut because they are correct: their rules
	// already work, or the faction can never satisfy them. Counted rather than
	// silently dropped — a report that hides what it excluded is how a reader
	// comes to trust a ranking more than it deserves.
	InertSites int
	Preempted  int
	Sites      []site
	Dead       []deadRule
	// Which blame sites track a doctrine input and which block regardless.
	Sensitivity []Sensitivity
	Doctrinal   int
	// Numeric gates worth moving, and what moving them would buy.
	Gates []Gate
	// Missing things followed back to whatever was supposed to make them.
	Chains []Chain
	// Rules the replay never saw satisfiable that the game says fired anyway.
	// The sample missed them, and calling them dead would be wrong.
	FiredAnyway []deadRule

	Home        string
	Windows     int
	Orphaned    int
	Approximate bool

	// Where the page fetches the model's reading from once it has loaded. Empty
	// when no model is configured.
	InsightURL string
	SweepURL   string
}

// buildWith adds the analysis that needs the windows kept apart.
func buildWith(title string, rep report, windows []windowStats, firings map[string]store.Firing, durationTicks int, faction string) view {
	v := build(title, rep, firings, durationTicks, faction)
	byRule := make(map[string][]clauseReport, len(rep.Rules))
	for _, r := range rep.Rules {
		byRule[r.Rule] = r.Clauses
	}
	v.Sensitivity = sensitivity(windows, v.Sites, byRule)
	v.Gates = gates(rep.Thresholds, rep.States)
	matched := make(map[string]int, len(firings))
	for name, f := range firings {
		matched[name] = f.Matched
	}
	v.Chains = chains(rep, v.Dead, matched)
	for _, s := range v.Sensitivity {
		if s.Doctrinal {
			v.Doctrinal++
		}
	}
	return v
}

// Rules the replay never saw fire but the game says did. The gap is the
// sampling rate, and saying "never satisfiable" about them would be wrong.
func reconcile(v *view, firings map[string]store.Firing) {
	for _, d := range v.Dead {
		if d.Acted > 0 {
			v.FiredAnyway = append(v.FiredAnyway, d)
		}
	}
}

// A rule whose first action lands after this share of the game is worth
// looking at. A quarter is early enough to catch an opener that never happened
// and late enough not to list every rule that needs a building first.
const lateStartPct = 25.0

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func build(title string, rep report, firings map[string]store.Firing, durationTicks int, faction string) view {
	v := view{Title: title, States: rep.States, RuleCount: len(rep.Rules)}

	// Aggregate by the line that wrote the requirement, across every rule it
	// appears in. Keyed on the text as well as the position so an inlined def
	// reads as one site rather than one per caller.
	byLine := map[string]*site{}
	for _, r := range rep.Rules {
		if r.Held == 0 {
			// A rule the sample never caught firing is not a rule that never
			// fired. The engine's own counters outrank the replay here: every
			// 15th evaluation is a thin sample, and counting a rule that
			// demonstrably worked as "never satisfiable" is how a reader is led
			// to go fix something that is not broken.
			if f, ok := firings[r.Rule]; ok && f.Acted > 0 {
				v.SampleMissed++
			} else {
				v.NeverFired++
				if r.Culprit == nil {
					v.NoCulprit++
				}
			}
		}
		if r.Preempted > r.Blocked && r.Preempted > 0 {
			v.Preempted++
		}
		for _, c := range r.Clauses {
			if c.Sole == 0 {
				continue
			}
			k := fmt.Sprintf("%s:%d:%s", c.File, c.Line, c.Source)
			s, ok := byLine[k]
			if !ok {
				s = &site{File: c.File, Line: c.Line, Source: c.Source}
				byLine[k] = s
			}
			s.Sole += c.Sole
			s.Blocked += c.Blocked
			s.Rules = append(s.Rules, r.Rule)
		}
	}
	for _, s := range byLine {
		sort.Strings(s.Rules)
		for _, r := range s.Rules {
			if f, ok := firings[r]; ok && f.Acted > 0 {
				s.Working++
			}
		}
		v.Sites = append(v.Sites, *s)
	}
	// A line whose every rule already works is not a reason anything failed —
	// `not squad-exists(ground-attack)` scoring 98 per 100 states means the
	// squad EXISTS and the rule that forms it correctly declined to run twice.
	// Ranked purely by blame these fill the whole first page, which is how a
	// scouting bug that was really a war-factory bug survived two readings.
	// They stay in the report, below the lines that actually stopped something.
	for i := range v.Sites {
		v.Sites[i].Impossible = impossibleFor(v.Sites[i].Source, faction)
	}
	sort.SliceStable(v.Sites, func(i, j int) bool {
		ai := v.Sites[i].Live() || v.Sites[i].Impossible != ""
		aj := v.Sites[j].Live() || v.Sites[j].Impossible != ""
		if ai != aj {
			return aj
		}
		return v.Sites[i].Sole > v.Sites[j].Sole
	})
	for _, st := range v.Sites {
		if st.Live() || st.Impossible != "" {
			v.InertSites++
		}
	}
	if len(v.Sites) > 12 {
		v.Sites = v.Sites[:12]
	}
	maxSite := 1
	for _, s := range v.Sites {
		if s.Sole > maxSite {
			maxSite = s.Sole
		}
	}
	for i := range v.Sites {
		v.Sites[i].SolePct = 100 * float64(v.Sites[i].Sole) / float64(maxSite)
		if rep.States > 0 {
			// Per rule, not per state. `Sole` is summed over every rule the
			// line blocks, so dividing by states alone produced "165 of every
			// 100 states" for a line inlined into seven rules. The denominator
			// is the chances it had: one per rule per state.
			chances := rep.States * max(len(v.Sites[i].Rules), 1)
			v.Sites[i].SoleRate = 100 * float64(v.Sites[i].Sole) / float64(chances)
		}
		v.Sites[i].File = filepath.Base(v.Sites[i].File)
	}

	// Rules that never fired, ranked by how often one clause was solely
	// responsible — the ones a single edit would free.
	for _, r := range rep.Rules {
		if r.Held > 0 || r.Culprit == nil {
			continue
		}
		// A rule the engine recorded acting is not a rule that never fired. It
		// has its own section; listing it here too produced rows reading
		// "fired 5x" under the heading "Rules that never fired".
		if f, ok := firings[r.Rule]; ok && f.Acted > 0 {
			continue
		}
		d := deadRule{
			Name: r.Rule, Category: r.Category, Seen: r.Seen,
			Blocked: r.Blocked, Preempted: r.Preempted,
			Matched: -1, Acted: -1,
		}
		if f, ok := firings[r.Rule]; ok {
			d.Matched, d.Acted = f.Matched, f.Acted
		}
		cul := r.Clauses[*r.Culprit]
		d.Culprit = &cul
		d.SolePct = 100 * float64(cul.Sole) / float64(max(r.Seen, 1))
		for _, c := range r.Clauses {
			d.Clauses = append(d.Clauses, clauseBar{
				clauseReport: c,
				SolePct:      100 * float64(c.Sole) / float64(max(r.Seen, 1)),
				OtherPct:     100 * float64(c.Blocked-c.Sole) / float64(max(r.Seen, 1)),
				IsCulprit:    c.Index == *r.Culprit,
				ShortFile:    filepath.Base(c.File),
			})
		}
		v.Dead = append(v.Dead, d)
	}
	sort.SliceStable(v.Dead, func(i, j int) bool { return v.Dead[i].Culprit.Sole > v.Dead[j].Culprit.Sole })
	if len(v.Dead) > 24 {
		v.Dead = v.Dead[:24]
	}
	// When each working rule first did something. Ranked latest-first, because
	// the interesting ones are the rules whose onset is the finding.
	v.DurationTicks = durationTicks
	v.Faction = faction
	if v.DurationTicks > 0 {
		for name, f := range firings {
			if f.Acted <= 0 || f.FirstTick <= 0 {
				continue
			}
			pct := 100 * float64(f.FirstTick) / float64(v.DurationTicks)
			l := LateStart{
				Name: name, Matched: f.Matched, Acted: f.Acted, Late: pct >= lateStartPct,
				FirstTick: f.FirstTick, LastTick: f.LastTick, Pct: pct,
				SpanPct: 100 * float64(f.LastTick-f.FirstTick) / float64(v.DurationTicks),
			}
			for _, r := range rep.Rules {
				if r.Rule == name {
					l.Category = r.Category
					break
				}
			}
			v.Late = append(v.Late, l)
		}
		// Ascending, so it reads as the game's own order: what opened, what
		// followed, and where the gaps are. Ranking by lateness instead puts
		// the reactive rules on top — flee-harvesters cannot fire before a
		// harvester is attacked — and buries the proactive rule that should
		// have gone first and did not.
		sort.Slice(v.Late, func(i, j int) bool {
			if v.Late[i].Pct != v.Late[j].Pct {
				return v.Late[i].Pct < v.Late[j].Pct
			}
			return v.Late[i].Name < v.Late[j].Name
		})
	}

	// Lead with the shapes that produced every real finding this tool has had:
	// the line that blocked most, the rule that fired and did nothing, and the
	// building that arrived latest.
	for _, st := range v.Sites {
		if st.Live() || st.Impossible != "" {
			continue
		}
		v.Findings = append(v.Findings, finding{
			Headline: fmt.Sprintf("%s was the only thing stopping a rule %.0f%% of the times it could have been",
				st.Source, st.SoleRate),
			Detail: fmt.Sprintf("%s:%d · blocks %d rule%s", st.File, st.Line, len(st.Rules), plural(len(st.Rules))),
		})
		break
	}
	var gapName string
	var gapMatched, gapActed int
	for name, f := range firings {
		if f.Acted < 0 || f.Matched-f.Acted <= gapMatched-gapActed {
			continue
		}
		gapName, gapMatched, gapActed = name, f.Matched, f.Acted
	}
	if gapName != "" && gapMatched-gapActed > 0 {
		v.Findings = append(v.Findings, finding{
			Headline: fmt.Sprintf("%s matched %d times and ordered nothing %d of them",
				gapName, gapMatched, gapMatched-gapActed),
			Detail: "a condition holding is not an action doing something",
		})
	}
	for i := len(v.Late) - 1; i >= 0; i-- {
		if !strings.HasPrefix(v.Late[i].Name, "build-") {
			continue
		}
		v.Findings = append(v.Findings, finding{
			Headline: fmt.Sprintf("%s first acted %.0f%% into the game", v.Late[i].Name, v.Late[i].Pct),
			Detail:   "the last building to arrive",
		})
		break
	}

	if len(v.Late) > 24 {
		v.LateOmitted = len(v.Late) - 24
		v.Late = v.Late[:24]
	}

	reconcile(&v, firings)
	return v
}
