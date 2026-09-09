package main

import (
	"regexp"
	"sort"
	"strings"

	"github.com/nstehr/vimy/vimy-core/rules"
)

// Following a missing thing back to whatever was supposed to make it.
//
// `count(idle-minelayers) > 0` is not a threshold, it is a report that no
// minelayer exists — and loosening or removing it changes nothing, because the
// problem is upstream. A blame count cannot tell "this gate is too tight" from
// "the thing does not exist", and readings of the same game kept recommending
// the former.
//
// So an existence test is followed one hop, to the rule whose action should have
// produced the thing. That moves the question from why the gate is closed to why
// nothing made one.

// existence matches clauses reporting a thing missing rather than a quantity
// being too small.
var existence = []*regexp.Regexp{
	regexp.MustCompile(`count\(([a-z0-9-]+)\)\s*(?:>\s*0|==\s*0|>=\s*1)`),
	regexp.MustCompile(`role-count\(([a-z0-9-]+)\)\s*(?:>\s*0|==\s*0|>=\s*1)`),
	regexp.MustCompile(`\bhas-role\(([a-z0-9-]+)\)`),
	regexp.MustCompile(`\bhas-unit\(([a-z0-9-]+)\)`),
	regexp.MustCompile(`\bcan-build-role\(([a-z0-9-]+)\)`),
}

// thingOf returns what a clause is asserting the existence of, or "" when the
// clause is not an existence test.
func thingOf(source string) string {
	for _, re := range existence {
		if m := re.FindStringSubmatch(source); m != nil {
			return thingName(m[1])
		}
	}
	return ""
}

// thingName reduces a predicate's subject to the noun a producing action would
// name: `idle-minelayers` and `minelayer` are the same thing.
func thingName(s string) string {
	for _, prefix := range []string{"idle-", "damaged-", "unassigned-idle-", "near-base-"} {
		s = strings.TrimPrefix(s, prefix)
	}
	s = strings.TrimSuffix(s, "s")
	return s
}

// Chain is a dead rule, the thing its blocking clause reports missing, and the
// rule that was supposed to make it.
type Chain struct {
	Rule   string
	Clause string
	File   string
	Line   int

	// What the clause reports missing.
	Thing string

	// The rule whose action would have produced it. Empty when no such rule
	// exists, which is itself the answer.
	Maker string
	// Whether that rule ever fired and did something in the real game.
	MakerMatched, MakerActed int
	// What stopped the maker, when it never fired.
	MakerCulprit string
	MakerFile    string
	MakerLine    int
}

// Verdict is the sentence to put next to it.
func (c Chain) Verdict() string {
	switch {
	case c.Maker == "":
		return "nothing in the rule set makes one"
	case c.MakerActed > 0:
		return "it was made, but not when the sample looked"
	case c.MakerCulprit != "":
		return "the rule that makes one was itself blocked"
	default:
		return "the rule that makes one never fired"
	}
}

// satisfiedByCompletion matches the guard a producing rule carries so it stops
// once it has succeeded. A maker held by one has done its job.
func satisfiedByCompletion(source string) bool {
	return strings.Contains(source, "not has-role(") ||
		strings.Contains(source, "not has-unit(") ||
		// A squad that exists is a forming rule that succeeded — read as a top
		// blocker otherwise, since it blocks every state after the first.
		strings.Contains(source, "not squad-exists(") ||
		// The scouting producer's guard: blocked by it means the enemy was found.
		strings.Contains(source, "not has-enemy-intel(") ||
		regexp.MustCompile(`role-count\([a-z0-9-]+\)\s*==\s*0`).MatchString(source) ||
		strings.Contains(source, "lost-role(")
}

// chains walks each dead rule's culprit clause back to whatever produces the
// thing it asks for.
func chains(rep report, dead []deadRule, firings map[string]int) []Chain {
	// Keyed on the noun in the action's name, not the rule's: a rule may be
	// called anything, while an action names what it does.
	makers := map[string]*ruleReport{}
	for i, r := range rep.Rules {
		for _, verb := range []string{"produce-", "build-", "rebuild-"} {
			if strings.HasPrefix(r.Action, verb) {
				thing := thingName(strings.TrimPrefix(r.Action, verb))
				// Source order, so a primary rule beats its rebuild- and extra-
				// variants.
				if _, seen := makers[thing]; !seen {
					makers[thing] = &rep.Rules[i]
				}
			}
		}
	}

	var out []Chain
	for _, d := range dead {
		if d.Culprit == nil || d.Acted > 0 {
			continue // it already works; the sample simply missed it
		}
		thing := thingOf(d.Culprit.Source)
		if thing == "" {
			continue // a real threshold, not an existence report
		}
		// The noun must be something the rules build. `idle-ground` reduces to
		// "ground", and reporting that nothing produces one is nonsense — without
		// this the section invents a missing thing per collection predicate.
		if !rules.IsRole(thing) {
			continue
		}
		c := Chain{
			Rule: d.Name, Clause: d.Culprit.Source,
			File: d.Culprit.File, Line: d.Culprit.Line, Thing: thing,
		}
		if m, ok := makers[thing]; ok {
			// A maker blocked only by "the thing already exists" has finished,
			// not stalled — `recover-mcv` falls silent once the MCV is deployed.
			if m.Culprit != nil && *m.Culprit < len(m.Clauses) &&
				satisfiedByCompletion(m.Clauses[*m.Culprit].Source) {
				continue
			}
			c.Maker = m.Rule
			c.MakerMatched, c.MakerActed = -1, -1
			if n, ok := firings[m.Rule]; ok {
				c.MakerMatched = n
			}
			if m.Culprit != nil && *m.Culprit < len(m.Clauses) {
				cl := m.Clauses[*m.Culprit]
				c.MakerCulprit, c.MakerFile, c.MakerLine = cl.Source, cl.File, cl.Line
			}
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return (out[i].MakerCulprit != "") && (out[j].MakerCulprit == "")
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

// --- Clauses this faction can never satisfy ---
//
// An Allied game's top blame sites were all support-power-ready on Soviet
// powers, blocking nearly every state. Correct and inert: the rule cannot fire
// and nothing about it should change.
//
// Blame cannot tell a gate that is too tight from one held shut all game by
// something no doctrine controls, so these rank as the tightest constraints in
// the game. Third kind of noise, after succeeded guards and sampling misses.

var supportPowerClause = regexp.MustCompile(`support-power-ready\(([A-Za-z]+)\)`)
var roleClause = regexp.MustCompile(`(?:has-role|can-build-role|role-count)\(([a-z0-9-]+)\)`)

// impossibleFor returns what a clause asks for that this faction can never
// have, or "" when the clause is satisfiable in principle.
func impossibleFor(source, faction string) string {
	if faction == "" {
		return ""
	}
	if m := supportPowerClause.FindStringSubmatch(source); m != nil {
		if !rules.SupportPowerReachable(m[1], faction) {
			return m[1]
		}
	}
	for _, m := range roleClause.FindAllStringSubmatch(source, -1) {
		role := strings.ReplaceAll(m[1], "-", "_")
		// Positive mentions only: `not has-role(x)` is satisfied by never having
		// x, which is the opposite of impossible.
		if strings.Contains(source, "not has-role("+m[1]+")") {
			continue
		}
		if !rules.BuildableByFaction(role, faction) {
			return m[1]
		}
	}
	return ""
}
