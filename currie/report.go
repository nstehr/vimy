package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
)

// A source line, and every rule it stops.
//
// The most useful axis in the report. A `require` written once inside a def is
// inlined into dozens of rules, so one line can stand between the AI and a third
// of its behaviour — which per-rule counts never show.
type site struct {
	File    string
	Line    int
	Source  string
	Sole    int
	Blocked int
	Rules   []string
	SolePct float64
	// Sole as a share of states replayed. Raw counts aren't comparable across
	// games: a shorter game yields fewer of everything, and the length difference
	// then reads as a finding.
	SoleRate float64
	// The doctrine axis every rule on this line serves, when they all serve one
	// the doctrine kept near zero. Such a line is the rule set obeying orders,
	// not a constraint worth relaxing.
	QuietAxis string
	QuietAt   float64
	// What this line asks for that the faction can never have; empty when the
	// clause is satisfiable in principle. Such a line is correct and inert.
	Impossible string
	// How many of the blocked rules already act in the real game. If they all do,
	// this line is not the problem — the sample simply missed them firing.
	Working int
}

// Live reports whether every rule this line blocks already works.
func (s site) Live() bool { return s.Working > 0 && s.Working == len(s.Rules) }

// Guard reports a completion guard — the clause a producing rule carries so it
// stops once it has succeeded. Being blocked by one is success.
//
// Separate from Live, which asks whether the blocked rules acted in this game's
// counters: a squad formed in an earlier doctrine window leaves its forming rule
// with no acts here while its guard still ranks as a top blocker.
func (s site) Guard() bool { return satisfiedByCompletion(s.Source) }

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
	// What the rule did in the real game; Acted is -1 when unmeasured.
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
// A blame count has no clock, so the replay can say what blocked a rule but not
// when a working one did its work. That gap made a scouting rule which fired
// seven times indistinguishable from a broken one, when the real finding was
// that nothing scouted until the game was half over.
type LateStart struct {
	Name     string
	Category string
	Matched  int
	Acted    int
	// Ticks, and how far into the game that is.
	FirstTick, LastTick int
	Pct                 float64
	// Marks rather than filters — a reactive rule is legitimately late.
	Late bool
	// First to last action as a share of the game: acting once is a different
	// thing from acting throughout.
	SpanPct float64
}

// finding is one fact worth reading first. With nine equally-weighted sections
// the reader reconstructs the same three questions every time — what blocked the
// most, what fired and achieved nothing, what arrived too late — so those are
// computed here and the page leads with them.
//
// Facts, not conclusions: "build-war-factory first acted at 57%" is what the
// data says; that this was too late is the reader's judgement.
type finding struct {
	Headline string
	Detail   string
}

// noOp is a rule whose condition held and whose action did nothing.
//
// The axis blame analysis cannot see. Currie replays CONDITIONS, so a rule that
// matched and then achieved nothing looks like a rule that worked. Five of the
// findings on 8-9 September were exactly this shape — FormSquad refusing to
// form a partial squad, deploy-mcv retrying one tile, repair-buildings holding
// a cash floor the rule did not state, guard-harvesters and squad-disengage
// both asking for idle members of a squad that was busy fighting — and every
// one was found by hand in SQL rather than by this report.
type noOp struct {
	Name    string
	Matched int
	Acted   int
	Gap     int
	// Share of matches that ordered something. Zero is the interesting case:
	// the rule fires and the action is inert.
	ActRate float64
	// The complement, as a percentage, for the bar.
	WastePct float64
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
	// Game duration, for the timeline axis.
	DurationTicks int
	// Who we played, so a clause can say which faction can never satisfy it.
	Faction string
	// The two or three facts worth reading before anything else.
	Findings []finding
	// Rules that matched and did nothing, worst first.
	NoOps []noOp
	// Working rules the timeline had no room for. Counted, because a silent cap
	// reads as "this is all of them".
	LateOmitted int
	// Blame sites cut for being correct — their rules already work, or the
	// faction can never satisfy them. Counted rather than dropped: a report that
	// hides its exclusions earns more trust than it deserves.
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
	// Rules the replay never saw satisfiable that the game says fired. The sample
	// missed them; calling them dead would be wrong.
	FiredAnyway []deadRule

	Home        string
	Windows     int
	Orphaned    int
	Approximate bool

	// Where the page fetches the model's reading once loaded; empty when no model
	// is configured.
	InsightURL string
	SweepURL   string
}

// buildWith adds the analysis that needs the windows kept apart.
func buildWith(title string, rep report, windows []windowStats, firings map[string]store.Firing, durationTicks int, faction string, doctrines []rules.Doctrine) view {
	v := build(title, rep, firings, durationTicks, faction, doctrines)
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

// Rules the replay never saw fire but the game says did — a sampling artifact,
// not evidence they are unsatisfiable.
func reconcile(v *view, firings map[string]store.Firing) {
	for _, d := range v.Dead {
		if d.Acted > 0 {
			v.FiredAnyway = append(v.FiredAnyway, d)
		}
	}
}

// Where a first action counts as late: early enough to catch an opener that
// never happened, late enough not to list every rule that needs a building.
const lateStartPct = 25.0

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func build(title string, rep report, firings map[string]store.Firing, durationTicks int, faction string, doctrines []rules.Doctrine) view {
	v := view{Title: title, States: rep.States, RuleCount: len(rep.Rules)}

	// Keyed on text as well as position, so an inlined def reads as one site
	// rather than one per caller.
	byLine := map[string]*site{}
	for _, r := range rep.Rules {
		if r.Held == 0 {
			// The engine's counters outrank the replay: every 15th evaluation is a
			// thin sample, and calling a rule that demonstrably worked "never
			// satisfiable" sends the reader to fix something that isn't broken.
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
	// A line whose rules all already work explains nothing: `not
	// squad-exists(ground-attack)` blocking almost every state means the squad
	// exists and its forming rule correctly declined to run twice. Ranked by
	// blame alone these fill the first page. They stay, below the lines that
	// actually stopped something.
	quiet := quietAxes(doctrines)
	for i := range v.Sites {
		v.Sites[i].Impossible = impossibleFor(v.Sites[i].Source, faction)
		// Only when EVERY rule the line blocks serves the same switched-off
		// axis. A shared def like reserves() blocks rules across the whole set,
		// and one quiet caller must not excuse it.
		axis, all := "", len(v.Sites[i].Rules) > 0
		for _, r := range v.Sites[i].Rules {
			a := axisOf(r)
			if a == "" {
				all = false
				break
			}
			if axis == "" {
				axis = a
			} else if axis != a {
				all = false
				break
			}
		}
		if all {
			if med, ok := quiet[axis]; ok {
				v.Sites[i].QuietAxis, v.Sites[i].QuietAt = axis, med
			}
		}
	}
	sort.SliceStable(v.Sites, func(i, j int) bool {
		ai := v.Sites[i].Live() || v.Sites[i].Guard() || v.Sites[i].Impossible != "" || v.Sites[i].QuietAxis != ""
		aj := v.Sites[j].Live() || v.Sites[j].Guard() || v.Sites[j].Impossible != "" || v.Sites[j].QuietAxis != ""
		if ai != aj {
			return aj
		}
		return v.Sites[i].Sole > v.Sites[j].Sole
	})
	for _, st := range v.Sites {
		if st.Live() || st.Guard() || st.Impossible != "" || st.QuietAxis != "" {
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
			// Sole sums over every rule the line blocks, so the denominator is
			// the chances it had — one per rule per state, not one per state.
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
		// A rule the engine recorded acting has its own section; listing it here
		// too yields rows reading "fired 5x" under "Rules that never fired".
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
		// Ascending, so it reads as the game's own order — what opened, what
		// followed, where the gaps are. Ranking by lateness floats the reactive
		// rules, which are legitimately late, above the proactive one that isn't.
		sort.Slice(v.Late, func(i, j int) bool {
			if v.Late[i].Pct != v.Late[j].Pct {
				return v.Late[i].Pct < v.Late[j].Pct
			}
			return v.Late[i].Name < v.Late[j].Name
		})
	}

	// The three shapes that have produced every real finding so far: the line
	// that blocked most, the rule that fired and achieved nothing, and the
	// building that arrived latest.
	for _, st := range v.Sites {
		if st.Live() || st.Guard() || st.Impossible != "" || st.QuietAxis != "" {
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
		// A building the doctrine deprioritised arriving late is obedience.
		// Game 95 led with "build-airfield first acted 73% into the game" on a
		// doctrine that held air_weight at 0.05 all game.
		if a := axisOf(v.Late[i].Name); a != "" {
			if _, quiet := quietAxes(doctrines)[a]; quiet {
				continue
			}
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

	for name, f := range firings {
		if f.Acted < 0 || f.Matched <= 0 || f.Matched == f.Acted {
			continue
		}
		v.NoOps = append(v.NoOps, noOp{
			Name: name, Matched: f.Matched, Acted: f.Acted,
			Gap:      f.Matched - f.Acted,
			ActRate:  float64(f.Acted) / float64(f.Matched),
			WastePct: 100 * (1 - float64(f.Acted)/float64(f.Matched)),
		})
	}
	// Rules that never acted at all first, then by how much work was wasted.
	// Ranking on the gap alone buries a rule that failed thirteen times under
	// one that failed nine hundred, and in game 91 the thirteen were the ones
	// that decided it.
	sort.Slice(v.NoOps, func(i, j int) bool {
		zi, zj := v.NoOps[i].Acted == 0, v.NoOps[j].Acted == 0
		if zi != zj {
			return zi
		}
		if v.NoOps[i].ActRate != v.NoOps[j].ActRate {
			return v.NoOps[i].ActRate < v.NoOps[j].ActRate
		}
		return v.NoOps[i].Gap > v.NoOps[j].Gap
	})
	if len(v.NoOps) > 10 {
		v.NoOps = v.NoOps[:10]
	}

	reconcile(&v, firings)
	return v
}
