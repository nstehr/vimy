package main

import (
	"regexp"
	"sort"
	"strings"

	"github.com/nstehr/vimy/vimy-core/rules"
)

// Following a missing thing back to whatever was supposed to make it.
//
// A clause like `count(idle-minelayers) > 0` is not a threshold. It is a report
// that no minelayer exists, and relaxing it — to "available" rather than "idle",
// or by removing it — changes nothing, because the problem is upstream. Three
// separate readings of the same game recommended exactly that, on three
// different rules, because a blame count cannot tell the difference between "the
// gate is too tight" and "the thing does not exist".
//
// So when the clause that stopped a rule is an existence test, this looks for
// the rule whose action was supposed to produce that thing, and reports on it
// instead. One hop is enough: it moves the question from "why is this gate
// closed" to "why did nothing make one", which is the question worth asking.

// existence matches the clause shapes that report a thing missing rather than a
// quantity being too small.
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
	// The rule that could not fire, and the clause that stopped it.
	Rule   string
	Clause string
	File   string
	Line   int

	// What the clause reports missing.
	Thing string

	// The rule whose action would have produced it. Empty when the rule set has
	// no such rule, which is itself the answer.
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

// satisfiedByCompletion reports whether a clause reads "the thing does not
// exist yet" — the guard a producing rule carries so it stops once it has
// succeeded. A maker held by one of these has done its job.
func satisfiedByCompletion(source string) bool {
	return strings.Contains(source, "not has-role(") ||
		strings.Contains(source, "not has-unit(") ||
		regexp.MustCompile(`role-count\([a-z0-9-]+\)\s*==\s*0`).MatchString(source) ||
		strings.Contains(source, "lost-role(")
}

// chains walks each dead rule's culprit clause back to whatever produces the
// thing it asks for.
func chains(rep report, dead []deadRule, firings map[string]int) []Chain {
	// What each action produces, by the noun in its name: `produce-minelayer`
	// makes a minelayer. Reading the action rather than the rule name because a
	// rule may be called anything, while an action names what it does.
	makers := map[string]*ruleReport{}
	for i, r := range rep.Rules {
		for _, verb := range []string{"produce-", "build-", "rebuild-"} {
			if strings.HasPrefix(r.Action, verb) {
				thing := thingName(strings.TrimPrefix(r.Action, verb))
				// The first rule wins, and rules arrive in source order, so a
				// primary rule beats its rebuild- and extra- variants.
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
		// The noun has to be something the rules can build. `idle-ground` reduces
		// to "ground", and reporting that no rule produces one is nonsense — it
		// is a quantity of units, not a unit. Without this check the section
		// invents a missing thing for every collection predicate, which is the
		// same over-claiming it was built to stop.
		if !rules.IsRole(thing) {
			continue
		}
		c := Chain{
			Rule: d.Name, Clause: d.Culprit.Source,
			File: d.Culprit.File, Line: d.Culprit.Line, Thing: thing,
		}
		if m, ok := makers[thing]; ok {
			// A maker that is itself blocked only by "the thing already
			// exists" is not blocked, it has finished. `recover-mcv` requires
			// `not has-role(construction-yard)`, so once the MCV is deployed
			// both it and `deploy-mcv` fall silent — which is the deployment
			// having worked, not a chain to follow.
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
	// The ones with a named cause first: they are the ones that lead somewhere.
	sort.SliceStable(out, func(i, j int) bool {
		return (out[i].MakerCulprit != "") && (out[j].MakerCulprit == "")
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}
