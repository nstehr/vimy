package main

import (
	"fmt"
	"sort"
)

// Asking what a doctrine input the strategist never tried would have done.
//
// Sensitivity reads only the variation the strategist produced: if tech-priority
// never went below 0.55, no reading of the archive says what 0.2 would do. A
// sweep replays the recorded states with one input overridden and every other
// held at what that window really had.
//
// The limit matters and is easy to forget. These states came from the game that
// happened, so a sweep says a rule would have been *satisfiable* in the
// situations that arose — not that the game would have gone differently, since
// different rules produce different situations from the first tick. A lower
// bound, not a prediction.

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
	// What the strategist actually used, so the page can mark the extrapolated
	// part of the sweep.
	UsedLow, UsedHigh float64
	// Live at the best and worst points, for the summary line.
	Best, Worst SweepPoint
}

// material is how far a sweep must move the held count to be worth naming a best
// for. One rule flickering in and out of reach across the range is noise.
const material = 0.05

// Verdict is the sentence to put next to the numbers, judged on held states: a
// rule satisfiable once and one satisfiable four hundred times are both "live",
// which hides most of what a sweep finds.
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

// sweepValues is fixed rather than derived from the data: the question is what
// the strategist did not try, so the grid must include what it never reached.
var sweepValues = []float64{0.1, 0.3, 0.5, 0.7, 0.9}

// sweep replays the game with one input overridden at each value, every other
// input held at what its window really had — so it measures one knob, not one
// doctrine.
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

// sweepKnobs limits the sweep to inputs sensitivity already implicates, so it
// costs a handful of runs rather than one per doctrine input.
func sweepKnobs(sens []Sensitivity, params map[string]float64) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range sens {
		if !s.Doctrinal || s.Knob == "" || seen[s.Knob] {
			continue
		}
		// Fractions only. Group sizes and prefers-* flags are not points on a
		// 0-to-1 line, and sweeping them there tries meaningless values.
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
