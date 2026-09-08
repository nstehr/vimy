package main

import "testing"

func win(name string, states int, blocked map[string]int, params map[string]float64) windowStats {
	return windowStats{Doctrine: name, Params: params, States: states, Blocked: blocked}
}

func siteFor(rule string) (site, map[string][]clauseReport) {
	st := site{File: "economy.vy", Line: 91, Source: "require cash >= 600", Rules: []string{rule}}
	by := map[string][]clauseReport{
		rule: {{Index: 0, File: "rules/vy/economy.vy", Line: 91, Source: "require cash >= 600"}},
	}
	return st, by
}

// A clause that blocks under high tech and not under low tech is the doctrine's
// doing, not the rule's.
func TestSensitivityNamesTheDoctrineInput(t *testing.T) {
	st, by := siteFor("produce-vehicle")
	key := clauseKey("produce-vehicle", 0)
	var ws []windowStats
	for i := 0; i < 4; i++ {
		ws = append(ws, win("cheap", 100, map[string]int{key: 10}, map[string]float64{"tech-priority": 0.3, "aggression": 0.5}))
	}
	for i := 0; i < 4; i++ {
		ws = append(ws, win("teched", 100, map[string]int{key: 95}, map[string]float64{"tech-priority": 0.8, "aggression": 0.5}))
	}

	out := sensitivity(ws, []site{st}, by)
	if len(out) != 1 {
		t.Fatalf("results = %d, want 1", len(out))
	}
	s := out[0]
	if !s.Doctrinal {
		t.Errorf("doctrinal = false, want true — 10%% against 95%% is not the rule's fault")
	}
	if s.Knob != "tech-priority" {
		t.Errorf("knob = %q, want tech-priority — aggression never moved", s.Knob)
	}
	if s.RateHigh <= s.RateLow {
		t.Errorf("rates = %.2f/%.2f, want high > low", s.RateLow, s.RateHigh)
	}
}

// A clause that blocks the same amount whatever the doctrine is the rule's own
// problem, and naming an input for it would be a false positive.
func TestSensitivityCallsAConstantBlockerTheRule(t *testing.T) {
	st, by := siteFor("produce-vehicle")
	key := clauseKey("produce-vehicle", 0)
	var ws []windowStats
	for i := 0; i < 8; i++ {
		ws = append(ws, win("w", 100, map[string]int{key: 90},
			map[string]float64{"tech-priority": 0.3 + float64(i)*0.05}))
	}

	out := sensitivity(ws, []site{st}, by)
	if len(out) != 1 {
		t.Fatalf("results = %d, want 1", len(out))
	}
	if out[0].Doctrinal {
		t.Errorf("doctrinal = true, want false — it blocks 90%% under every doctrine")
	}
	if got := out[0].Verdict(); got != "blocks under every doctrine" {
		t.Errorf("verdict = %q", got)
	}
}

// Too few doctrines is not an experiment, and reporting one would invite a
// conclusion the data cannot carry.
func TestSensitivityDeclinesWithTooFewWindows(t *testing.T) {
	st, by := siteFor("produce-vehicle")
	key := clauseKey("produce-vehicle", 0)
	ws := []windowStats{
		win("a", 100, map[string]int{key: 10}, map[string]float64{"tech-priority": 0.3}),
		win("b", 100, map[string]int{key: 90}, map[string]float64{"tech-priority": 0.8}),
	}
	if out := sensitivity(ws, []site{st}, by); out != nil {
		t.Errorf("results = %d, want none from 2 windows", len(out))
	}
}

// An input the strategist never moved cannot explain anything.
func TestSensitivityIgnoresAnInputThatNeverVaried(t *testing.T) {
	st, by := siteFor("produce-vehicle")
	key := clauseKey("produce-vehicle", 0)
	var ws []windowStats
	for i := 0; i < 8; i++ {
		blocked := 10
		if i >= 4 {
			blocked = 95
		}
		ws = append(ws, win("w", 100, map[string]int{key: blocked},
			map[string]float64{"naval-weight": 0.0, "tech-priority": 0.3 + float64(i)*0.1}))
	}

	out := sensitivity(ws, []site{st}, by)
	if out[0].Knob == "naval-weight" {
		t.Error("knob = naval-weight, which was 0 in every window")
	}
}
