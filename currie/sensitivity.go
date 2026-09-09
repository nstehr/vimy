package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
)

// Telling a rule-set problem from a doctrine that priced itself out.
//
// A blame site says a `require` stopped a rule, not whose fault that is, and the
// two cases want opposite responses:
//
//   - Blocking about as much under every doctrine: the rule set asks for
//     something the game rarely provides. Change the rule.
//   - Blocking heavily under some doctrines and barely under others: the rules
//     are fine and the strategist priced itself out. Change the prompt, or the
//     weights it can reach.
//
// A game runs dozens of doctrines, each compiling the same sources to different
// thresholds, so the archive already contains the experiment.

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

	// The doctrine input best separating the windows where this clause blocked
	// from those where it did not; empty when nothing separates them.
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

// doctrinalSpread is the gap between halves worth naming; below it the clause
// blocks for reasons no doctrine controls.
const doctrinalSpread = 0.25

// minWindowsPerSide keeps a split from being decided by one short window.
const minWindowsPerSide = 3

// sensitivity ranks the clauses whose blocking is most explained by a doctrine
// input, and reports the rest as constant.
func sensitivity(windows []windowStats, sites []site, byRule map[string][]clauseReport) []Sensitivity {
	if len(windows) < 2*minWindowsPerSide {
		return nil // too few doctrines to say anything about variation
	}

	// Only inputs that actually varied: a weight the strategist never moved
	// explains nothing and only invites a false positive.
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

	// One row per site: a `require` inside a def is the same line at the same
	// threshold in every caller, and listing them separately buries the sites
	// that appear once.
	var out []Sensitivity
	for _, st := range sites {
		var keys []string
		for _, rule := range st.Rules {
			for _, c := range byRule[rule] {
				// site.File is a basename by template time; a clause still carries
				// its compiled path.
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

// analyseSite measures one source line across doctrine windows, averaging over
// the rules it appears in — a shared clause blocking five rules is one fact
// about the line, not five.
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

	// Median split rather than correlation: the doctrine inputs move together, so
	// a coefficient reads as significance it hasn't earned, and "blocks 34% below
	// 0.55 and 96% above" is a claim someone can check.
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
