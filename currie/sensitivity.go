package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
)

// Telling a rule-set problem from a doctrine that priced itself out.
//
// A blame site says a `require` stopped a rule. It does not say whose fault
// that is, and the two cases want opposite responses:
//
//   - The clause blocks about as much under every doctrine. The rule set asks
//     for something the game rarely provides — go and change the rule.
//   - The clause blocks heavily under some doctrines and barely under others.
//     The rules are fine and the strategist priced itself out — go and change
//     the prompt, or the weights it is allowed to reach.
//
// The same rule sources compile to different thresholds under different
// doctrines, and a game runs dozens of them, so the archive already contains the
// experiment. This reads it.

// Sensitivity is one clause, and what its blocking tracks.
type Sensitivity struct {
	Source string
	File   string
	Line   int
	// How many rules this line is inlined into, and how many doctrine windows
	// it was measured across.
	Rules   int
	Windows int

	// Block rate across windows, weighted by states.
	Low, High, Overall float64

	// The doctrine input whose value best separates the windows where this
	// clause blocked from the ones where it did not. Empty when nothing
	// separates them.
	Knob      string
	KnobLow   float64 // mean value of the knob in the low-blocking half
	KnobHigh  float64
	RateLow   float64 // block rate in the half where the knob is low
	RateHigh  float64
	Spread    float64 // |RateHigh - RateLow|, how much the knob moves it
	Doctrinal bool    // the spread is large enough to call it
}

// Percentages for the range bar, which spans 0–100% block rate.
func (s Sensitivity) LowPct() float64     { return s.Low * 100 }
func (s Sensitivity) SpanPct() float64    { return (s.High - s.Low) * 100 }
func (s Sensitivity) OverallPct() float64 { return s.Overall * 100 }

// Verdict is the sentence to put next to the number.
func (s Sensitivity) Verdict() string {
	switch {
	case s.Doctrinal && s.RateHigh > s.RateLow:
		return fmt.Sprintf("blocks when %s is high", s.Knob)
	case s.Doctrinal:
		return fmt.Sprintf("blocks when %s is low", s.Knob)
	case s.High-s.Low < 0.15:
		return "blocks under every doctrine"
	default:
		return "varies, but no single input explains it"
	}
}

// doctrinalSpread is how far apart the two halves must be before the difference
// is worth naming. Below this the clause is blocking for reasons the doctrine
// does not control.
const doctrinalSpread = 0.25

// minWindowsPerSide keeps a split from being decided by one short window.
const minWindowsPerSide = 3

// sensitivity ranks the clauses whose blocking is most explained by a doctrine
// input, and reports the rest as constant.
func sensitivity(windows []windowStats, sites []site, byRule map[string][]clauseReport) []Sensitivity {
	if len(windows) < 2*minWindowsPerSide {
		return nil // too few doctrines to say anything about variation
	}

	// Every doctrine input that actually varied. A weight the strategist never
	// moved explains nothing, and testing it only invites a false positive.
	knobs := map[string]bool{}
	for k := range windows[0].Params {
		first, varies := windows[0].Params[k], false
		for _, w := range windows[1:] {
			if w.Params[k] != first {
				varies = true
				break
			}
		}
		if varies {
			knobs[k] = true
		}
	}

	// One row per site, not per rule. A `require` written inside a def is
	// inlined into every rule that calls it, and those are the same line with
	// the same threshold — listing them separately says the same thing six
	// times and buries the sites that only appear once.
	var out []Sensitivity
	for _, st := range sites {
		var keys []string
		for _, rule := range st.Rules {
			for _, c := range byRule[rule] {
				// `site.File` is a basename by the time it reaches a template;
				// a clause still carries the path it was compiled from.
				if filepath.Base(c.File) != filepath.Base(st.File) ||
					c.Line != st.Line || c.Source != st.Source {
					continue
				}
				keys = append(keys, clauseKey(rule, c.Index))
				break
			}
		}
		if len(keys) == 0 {
			continue
		}
		s := analyseSite(windows, knobs, st, keys)
		if s.Windows >= 2*minWindowsPerSide {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Spread > out[j].Spread })
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

// analyseSite measures one source line across the doctrine windows.
//
// The rate is averaged over the rules the line appears in: a shared clause
// blocking five rules in a window is one fact about the line, not five.
func analyseSite(windows []windowStats, knobs map[string]bool, st site, keys []string) Sensitivity {
	type point struct {
		rate   float64
		states int
		params map[string]float64
	}
	var pts []point
	for _, w := range windows {
		if w.States == 0 {
			continue
		}
		blocked := 0
		for _, k := range keys {
			blocked += w.Blocked[k]
		}
		pts = append(pts, point{
			rate:   float64(blocked) / float64(w.States*len(keys)),
			states: w.States,
			params: w.Params,
		})
	}

	s := Sensitivity{
		Source: st.Source, File: st.File, Line: st.Line,
		Rules: len(keys), Windows: len(pts),
		Low: 1, High: 0,
	}
	var num, den float64
	for _, p := range pts {
		s.Low = math.Min(s.Low, p.rate)
		s.High = math.Max(s.High, p.rate)
		num += p.rate * float64(p.states)
		den += float64(p.states)
	}
	if den > 0 {
		s.Overall = num / den
	}

	// Split the windows at each knob's median and compare the halves. A median
	// split rather than a correlation: the doctrine inputs move together, so a
	// correlation coefficient reads as significance it has not earned, and
	// "blocks 34% below 0.55 and 96% above" is a claim someone can check.
	for k := range knobs {
		vals := make([]float64, 0, len(pts))
		for _, p := range pts {
			vals = append(vals, p.params[k])
		}
		sorted := append([]float64(nil), vals...)
		sort.Float64s(sorted)
		median := sorted[len(sorted)/2]

		var loRate, hiRate, loW, hiW, loKnob, hiKnob float64
		var loN, hiN int
		for _, p := range pts {
			w := float64(p.states)
			if p.params[k] < median {
				loRate += p.rate * w
				loKnob += p.params[k] * w
				loW += w
				loN++
			} else {
				hiRate += p.rate * w
				hiKnob += p.params[k] * w
				hiW += w
				hiN++
			}
		}
		if loN < minWindowsPerSide || hiN < minWindowsPerSide || loW == 0 || hiW == 0 {
			continue
		}
		loRate, hiRate = loRate/loW, hiRate/hiW
		if spread := math.Abs(hiRate - loRate); spread > s.Spread {
			s.Knob, s.Spread = k, spread
			s.RateLow, s.RateHigh = loRate, hiRate
			s.KnobLow, s.KnobHigh = loKnob/loW, hiKnob/hiW
		}
	}
	s.Doctrinal = s.Spread >= doctrinalSpread
	return s
}
