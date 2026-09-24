package main

import "testing"

// The loop must be bounded and must surface its trace even when it runs out of
// steps: eight tool results are worth reading when nothing was concluded from
// them, and an unbounded analyst is a bill.
func TestInvestigationIsBounded(t *testing.T) {
	if maxInvestigationSteps <= 0 || maxInvestigationSteps > 12 {
		t.Errorf("maxInvestigationSteps = %d, want a small positive bound", maxInvestigationSteps)
	}
}

// Every tool must say what it means when there is no telemetry, rather than
// returning empty and letting the model read absence as evidence.
func TestToolsDistinguishAbsentTelemetryFromQuiet(t *testing.T) {
	iv := &investigator{ch: nil}
	for name, got := range map[string]string{
		"squad_timeline":  iv.squadTimeline(t.Context(), ""),
		"strike_blockers": iv.strikeBlockers(t.Context(), ""),
		"field_at":        iv.fieldAt(t.Context(), "", 1000),
	} {
		if got != noTelemetry {
			t.Errorf("%s with no telemetry returned %q, want the explicit no-telemetry note", name, got)
		}
	}
}
