package main

import "testing"

func sweepOf(held ...int) Sweep {
	s := Sweep{Knob: "tech-priority", UsedLow: 0.5, UsedHigh: 0.6}
	for i, h := range held {
		s.Points = append(s.Points, SweepPoint{Value: sweepValues[i], Held: h, Live: 30})
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
	return s
}

// A sweep that barely moves is not an optimum, and naming one would be the
// overclaim this tool exists to avoid.
func TestSweepDoesNotNameABestForNoise(t *testing.T) {
	s := sweepOf(1455, 1457, 1457, 1457, 1456)
	if s.Material() {
		t.Error("material = true for a 0.1% spread")
	}
	if got := s.Verdict(); got != "changes almost nothing" {
		t.Errorf("verdict = %q", got)
	}
}

// A real difference outside the range the strategist used is the finding a
// sweep exists for: the archive alone cannot reach it.
func TestSweepNamesAValueOutsideWhatWasTried(t *testing.T) {
	s := sweepOf(1000, 1100, 1300, 1600, 1500)
	if !s.Material() {
		t.Fatal("material = false for a 60% spread")
	}
	if got := s.Verdict(); got != "best at 0.7, above anything tried" {
		t.Errorf("verdict = %q", got)
	}
}

// Only inputs the sensitivity read implicates, and only ones that are a
// fraction — a group size is not a point on a 0-to-1 line.
func TestSweepKnobsPicksOnlyImplicatedFractions(t *testing.T) {
	params := map[string]float64{
		"tech-priority": 0.7, "ground-attack-group-size": 8, "naval-weight": 0,
	}
	sens := []Sensitivity{
		{Knob: "tech-priority", Doctrinal: true},
		{Knob: "ground-attack-group-size", Doctrinal: true},
		{Knob: "naval-weight", Doctrinal: false},
		{Knob: "tech-priority", Doctrinal: true},
	}

	got := sweepKnobs(sens, params)
	if len(got) != 1 || got[0] != "tech-priority" {
		t.Errorf("knobs = %v, want [tech-priority]", got)
	}
}
