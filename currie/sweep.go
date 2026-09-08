package main

import (
	"fmt"
	"sort"
)

// Asking what a doctrine input the strategist never tried would have done.
//
// The sensitivity analysis reads variation the strategist actually produced, so
// it can only speak about values it chose. If `tech-priority` never went below
// 0.55 all game, no amount of reading the archive says what 0.2 would have
// done. A sweep replays the recorded states with one input overridden, holding
// every other input at what that window really had.
//
// The limit is worth stating plainly, because it is easy to forget: these
// states came from the game that happened. A sweep says whether a rule would
// have been *satisfiable* in the situations that arose, not that the game would
// have gone differently — different rules produce different situations from the
// first tick. It is a lower bound on what changed, not a prediction.

// SweepPoint is one value of one input, and what the rule set could do there.
type SweepPoint struct {
	Value float64
	// Rules that could fire in at least one state.
	Live int
	// States, summed over rules, where every requirement held.
	Held int
	// True for the value closest to what the doctrines actually used.
	Actual bool
}

// Sweep is one doctrine input, tried across a range.
type Sweep struct {
	Knob   string
	Points []SweepPoint
	// The range the strategist actually used, so the page can say which part of
	// the sweep is extrapolation.
	UsedLow, UsedHigh float64
	// Live at the best and worst points, for the summary line.
	Best, Worst SweepPoint
}

// material is how much a sweep must move the held count before the difference
// is worth naming a best for. One rule flickering in and out of reach across a
// whole range is noise, and calling it an optimum would be the same overclaim
// this tool exists to avoid.
const material = 0.05

// Verdict is the sentence to put next to the numbers.
//
// Judged on held states rather than live rules: a rule satisfiable once and a
// rule satisfiable four hundred times both count as live, so the coarser
// measure hides most of what a sweep finds.
func (s Sweep) Verdict() string {
	if s.Worst.Held == 0 || float64(s.Best.Held-s.Worst.Held)/float64(s.Worst.Held) < material {
		return "changes almost nothing"
	}
	switch {
	case s.Best.Value < s.UsedLow:
		return fmt.Sprintf("best at %.1f, below anything tried", s.Best.Value)
	case s.Best.Value > s.UsedHigh:
		return fmt.Sprintf("best at %.1f, above anything tried", s.Best.Value)
	default:
		return fmt.Sprintf("best at %.1f, inside the range used", s.Best.Value)
	}
}

// Material reports whether the sweep found a difference worth acting on.
func (s Sweep) Material() bool {
	return s.Worst.Held > 0 && float64(s.Best.Held-s.Worst.Held)/float64(s.Worst.Held) >= material
}

// sweepValues are the points every float input is tried at. Fixed rather than
// derived: the question is what the strategist did not try, so the grid has to
// include values it never reached.
var sweepValues = []float64{0.1, 0.3, 0.5, 0.7, 0.9}

// sweep replays the game with one input overridden at each value.
//
// Every other input keeps the value its own window really had, so this measures
// one knob rather than one doctrine.
func sweep(knobs []string, windows []windowStats, run func(map[string]float64, int) (report, error)) ([]Sweep, error) {
	var out []Sweep
	for _, knob := range knobs {
		s := Sweep{Knob: knob, UsedLow: 1e9, UsedHigh: -1e9}
		for _, w := range windows {
			v := w.Params[knob]
			s.UsedLow, s.UsedHigh = min(s.UsedLow, v), max(s.UsedHigh, v)
		}

		for _, v := range sweepValues {
			pt := SweepPoint{Value: v}
			live := map[string]bool{}
			for i, w := range windows {
				params := make(map[string]float64, len(w.Params))
				for k, val := range w.Params {
					params[k] = val
				}
				params[knob] = v
				rep, err := run(params, i)
				if err != nil {
					return nil, err
				}
				for _, r := range rep.Rules {
					if r.Held > 0 {
						live[r.Rule] = true
						pt.Held += r.Held
					}
				}
			}
			pt.Live = len(live)
			pt.Actual = v >= s.UsedLow && v <= s.UsedHigh
			s.Points = append(s.Points, pt)
		}

		s.Best, s.Worst = s.Points[0], s.Points[0]
		for _, p := range s.Points {
			if p.Held > s.Best.Held {
				s.Best = p
			}
			if p.Held < s.Worst.Held {
				s.Worst = p
			}
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Best.Held-out[i].Worst.Held > out[j].Best.Held-out[j].Worst.Held
	})
	return out, nil
}

// sweepKnobs picks what is worth trying: the inputs the sensitivity analysis
// already implicates, which keeps a sweep to a handful of runs rather than one
// per doctrine input.
func sweepKnobs(sens []Sensitivity, params map[string]float64) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range sens {
		if !s.Doctrinal || s.Knob == "" || seen[s.Knob] {
			continue
		}
		// Only inputs that are a fraction. Group sizes and the prefers-* flags
		// are not points on a 0-to-1 line, and sweeping them there would try
		// values that mean nothing.
		if v, ok := params[s.Knob]; !ok || v < 0 || v > 1 {
			continue
		}
		seen[s.Knob] = true
		out = append(out, s.Knob)
		if len(out) == 3 {
			break
		}
	}
	return out
}
