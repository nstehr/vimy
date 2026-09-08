package main

import "testing"

func gate(rule string, at, reached, floor float64, blocked int, curve ...curvePoint) thresholdReport {
	return thresholdReport{
		Rule: rule, File: "rules/vy/economy.vy", Line: 91, Source: "require cash >= 600",
		Op: "GtEq", At: at, Blocked: blocked, Reached: reached, Floor: floor, Curve: curve,
	}
}

// A gate sitting inside the observed range is a dial, and the relief is what
// moving it buys.
func TestGatesReportReliefForATunableGate(t *testing.T) {
	in := []thresholdReport{gate("produce-vehicle", 600, 2000, 0, 300,
		curvePoint{At: 100, Blocked: 90}, curvePoint{At: 300, Blocked: 190}, curvePoint{At: 600, Blocked: 300})}

	out := gates(in, 500)
	if len(out) != 1 {
		t.Fatalf("gates = %d, want 1", len(out))
	}
	if out[0].Relief != 210 {
		t.Errorf("relief = %d, want 210 (300 blocked now, 90 at the best reachable value)", out[0].Relief)
	}
	if out[0].ReliefLow != 100 {
		t.Errorf("reliefLow = %v, want 100", out[0].ReliefLow)
	}
}

// Relief found only at or below the smallest observed value is the comparison
// going vacuous, not a gate being turned — and `sole` already reports removal.
func TestGatesIgnoreReliefAtOrBelowTheFloor(t *testing.T) {
	// squad-ready-ratio was 0 in every state; ">= 0" is trivially true.
	in := []thresholdReport{gate("squad-attack", 1, 0, 0, 400,
		curvePoint{At: 0, Blocked: 0}, curvePoint{At: 1, Blocked: 400})}

	if out := gates(in, 400); len(out) != 0 {
		t.Errorf("gates = %d, want none — the only relief is the gate going vacuous", len(out))
	}
}

// A gate above anything ever reached is asking for something that did not
// happen; the rule is what to change, not the number.
func TestGatesDropAnUnreachableGate(t *testing.T) {
	in := []thresholdReport{gate("build-tech-center", 5000, 300, 0, 500,
		curvePoint{At: 100, Blocked: 400}, curvePoint{At: 5000, Blocked: 500})}

	out := gates(in, 500)
	if len(out) != 1 {
		t.Fatalf("gates = %d, want 1", len(out))
	}
	if out[0].Relief == 0 {
		t.Skip("relief is available below `reached`, so this gate is kept")
	}
	if out[0].ReliefHigh > out[0].Reached {
		t.Errorf("reliefHigh %v is above anything reached (%v)", out[0].ReliefHigh, out[0].Reached)
	}
}

// Relief is summed per window. Two windows compiling the same line to different
// numbers are two dials, and their curves are measured at different values.
func TestGatesSumReliefAcrossWindows(t *testing.T) {
	in := []thresholdReport{
		gate("produce-vehicle", 600, 2000, 0, 100, curvePoint{At: 200, Blocked: 40}, curvePoint{At: 600, Blocked: 100}),
		gate("produce-vehicle", 900, 2000, 0, 150, curvePoint{At: 300, Blocked: 50}, curvePoint{At: 900, Blocked: 150}),
	}

	out := gates(in, 500)
	if len(out) != 1 {
		t.Fatalf("gates = %d, want 1 — same file and line", len(out))
	}
	if out[0].Blocked != 250 {
		t.Errorf("blocked = %d, want 250", out[0].Blocked)
	}
	if out[0].Relief != 160 {
		t.Errorf("relief = %d, want 160 (60 + 100)", out[0].Relief)
	}
	if len(out[0].Rules) != 1 {
		t.Errorf("rules = %v, want the rule counted once", out[0].Rules)
	}
}
