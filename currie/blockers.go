package main

import "path/filepath"

// Blocker is one clause line with everything the page knows about it.
//
// The page used to answer "what stopped things" five times over: the lines that
// blocked, the missing things upstream of them, the gates worth moving, the
// doctrine sensitivity of each line, and the per-rule clause breakdown. Four of
// those are keyed on the same thing - a file and a line - so a reader chasing
// one clause had to find it again in four places, each with its own ranking.
// They are one table now. The fifth, the per-rule breakdown, stays separate and
// folded: it is keyed on the rule rather than the line, and it is the drill-down
// you open once you know which line to chase.
type Blocker struct {
	// Sole-blame stats. Zero-valued for a line that only ever turns up as a
	// tunable gate, which is why Ranked exists.
	site
	// Ranked says this line was measured as a sole blocker. Unranked rows are
	// the gates and chains whose line did not make the sole-blame cut; they
	// carry real information and no standing.
	Ranked bool

	Gate   *Gate
	Sens   *Sensitivity
	Chains []Chain
}

// Tunable reports a gate whose number could move and buy something.
func (b Blocker) Tunable() bool { return b.Gate != nil && !b.Gate.Untunable() }

// Inert reports a line that is correct as written: a completion guard, a
// requirement the faction can never satisfy, a rule the doctrine told to stay
// quiet, or one whose rules already work.
func (b Blocker) Inert() bool {
	return b.Live() || b.Guard() || b.Impossible != "" || b.QuietAxis != ""
}

// maxExtraBlockers caps the rows that have no sole-blame standing. The ranked
// ones are already capped upstream.
const maxExtraBlockers = 8

type blockerKey struct {
	file string
	line int
}

func keyOf(file string, line int) blockerKey {
	return blockerKey{file: filepath.Base(file), line: line}
}

// blockers joins the line-keyed views into one ranked list.
func blockers(v *view) []Blocker {
	out := make([]Blocker, 0, len(v.Sites)+maxExtraBlockers)
	at := make(map[blockerKey]int, len(v.Sites))
	for _, s := range v.Sites {
		at[keyOf(s.File, s.Line)] = len(out)
		out = append(out, Blocker{site: s, Ranked: true})
	}

	// A line the sole-blame ranking did not reach still gets a row, because a
	// gate nobody can move is a finding even when no single state hangs on it.
	extra := 0
	add := func(file string, line int, source string) int {
		k := keyOf(file, line)
		if i, ok := at[k]; ok {
			return i
		}
		if extra >= maxExtraBlockers {
			return -1
		}
		extra++
		at[k] = len(out)
		out = append(out, Blocker{site: site{File: k.file, Line: line, Source: source}})
		return len(out) - 1
	}

	for i := range v.Gates {
		g := &v.Gates[i]
		if j := add(g.File, g.Line, g.Source); j >= 0 {
			out[j].Gate = g
		}
	}
	for i := range v.Sensitivity {
		s := &v.Sensitivity[i]
		if j := add(s.File, s.Line, s.Source); j >= 0 {
			out[j].Sens = s
		}
	}
	for _, c := range v.Chains {
		if j := add(c.File, c.Line, c.Clause); j >= 0 {
			out[j].Chains = append(out[j].Chains, c)
		}
	}
	return out
}
