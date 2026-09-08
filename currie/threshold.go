package main

import (
	"fmt"
	"path/filepath"
	"sort"
)

// What a numeric gate would have cost at another value.
//
// `sole` says removing a clause lets a rule fire. It does not say whether
// *moving* it would, and for a threshold that is the question worth asking. A
// gate at 600 against a treasury that never exceeded 50 is not mistuned — it is
// asking for something that never happened, and lowering it to 500 buys
// nothing. A gate sitting in the middle of the observed range is a dial.

type thresholdReport struct {
	Rule    string       `json:"rule"`
	Clause  int          `json:"clause"`
	File    string       `json:"file"`
	Line    int          `json:"line"`
	Source  string       `json:"source"`
	Op      string       `json:"op"`
	At      float64      `json:"at"`
	Blocked int          `json:"blocked"`
	Reached float64      `json:"reached"`
	Floor   float64      `json:"floor"`
	Curve   []curvePoint `json:"curve"`
}

type curvePoint struct {
	At      float64 `json:"at"`
	Blocked int     `json:"blocked"`
}

// Gate is one tunable threshold, ranked by what moving it would buy.
type Gate struct {
	File   string
	Line   int
	Source string
	Rules  []string
	// The value the gate had. A range, because the same line compiles to a
	// different number under every doctrine and this row spans all of them.
	AtLow, AtHigh float64
	Blocked       int
	Reached       float64
	Curve         []curvePoint

	// States that moving the gate, without leaving the observed range, would
	// stop blocking. Zero means the gate is not the dial it looks like.
	Relief int
	// The values that buy `Relief`, over the windows where any was available.
	ReliefLow, ReliefHigh float64
	// Blocked as a share of the states measured, for the bar.
	BlockedPct float64
	ReliefPct  float64
}

// Untunable reports a gate set beyond anything the measured side reached — the
// change worth making is to the rule, not to the number.
func (g Gate) Untunable() bool { return g.Relief == 0 }

// gates ranks the thresholds worth tuning, merging the ones that are the same
// line inlined into several rules.
func gates(in []thresholdReport, states int) []Gate {
	if states == 0 {
		return nil
	}
	type key struct {
		file string
		line int
	}
	merged := map[key]*Gate{}
	seen := map[key]map[string]bool{}
	var order []key

	for _, t := range in {
		// Relief per window, where the curve's candidates and the counts they
		// were measured at belong together.
		best := t.Blocked
		bestAt := t.At
		for _, p := range t.Curve {
			// Strictly inside the observed range. At or below the floor the
			// comparison stops excluding anything, and a gate that excludes
			// nothing is not a gate that was turned — it is one that was
			// removed, which `sole` already reports.
			if p.At < t.At && p.At <= t.Reached && p.At > t.Floor && p.Blocked < best {
				best, bestAt = p.Blocked, p.At
			}
		}

		k := key{filepath.Base(t.File), t.Line}
		g, ok := merged[k]
		if !ok {
			g = &Gate{
				File: filepath.Base(t.File), Line: t.Line, Source: t.Source,
				AtLow: t.At, AtHigh: t.At, ReliefLow: -1, ReliefHigh: -1,
			}
			merged[k] = g
			seen[k] = map[string]bool{}
			order = append(order, k)
		}
		g.AtLow, g.AtHigh = min(g.AtLow, t.At), max(g.AtHigh, t.At)
		if !seen[k][t.Rule] {
			seen[k][t.Rule] = true
			g.Rules = append(g.Rules, t.Rule)
		}
		g.Blocked += t.Blocked
		g.Relief += t.Blocked - best
		if t.Reached > g.Reached {
			g.Reached = t.Reached
		}
		if t.Blocked-best > 0 {
			if g.ReliefLow < 0 {
				g.ReliefLow, g.ReliefHigh = bestAt, bestAt
			}
			g.ReliefLow, g.ReliefHigh = min(g.ReliefLow, bestAt), max(g.ReliefHigh, bestAt)
		}
	}

	out := make([]Gate, 0, len(order))
	for _, k := range order {
		g := merged[k]
		span := float64(states * len(g.Rules))
		g.BlockedPct = 100 * float64(g.Blocked) / span
		g.ReliefPct = 100 * float64(g.Relief) / span
		sort.Strings(g.Rules)
		out = append(out, *g)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Relief != out[j].Relief {
			return out[i].Relief > out[j].Relief
		}
		return out[i].Blocked > out[j].Blocked
	})
	// Only the gates that are dials. A gate with no relief is already reported
	// by the blame section, where the honest reading of it lives.
	kept := out[:0]
	for _, g := range out {
		if g.Relief > 0 {
			kept = append(kept, g)
		}
	}
	if len(kept) > 10 {
		kept = kept[:10]
	}
	return kept
}

// Range renders a value that may differ per window as one string.
func rng(low, high float64) string {
	if low == high {
		return fmt.Sprintf("%.0f", low)
	}
	return fmt.Sprintf("%.0f–%.0f", low, high)
}

// At is the gate's value, as a range when the doctrines compiled it differently.
func (g Gate) At() string { return rng(g.AtLow, g.AtHigh) }

// MovedTo is the value that buys the relief, likewise.
func (g Gate) MovedTo() string {
	if g.ReliefLow < 0 {
		return "—"
	}
	return rng(g.ReliefLow, g.ReliefHigh)
}
