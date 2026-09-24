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

// A feed that was not running is not an empty map.
//
// The first live investigation concluded that game 174 lost for want of
// scouting, on the strength of this tool reporting an empty threat map. The
// threat emitter did not exist when that game was played: zero rows meant no
// feed, and the tool asserted it meant no intel. Absence of data presented as
// evidence - the exact error the tool menu warns the model about.
func TestEmptyFeedIsNotAnEmptyMap(t *testing.T) {
	iv := &investigator{ch: nil}
	got := iv.fieldAt(t.Context(), "", 1000)
	if got != noTelemetry {
		t.Fatalf("no client: %q", got)
	}
	// The distinction the tool must draw, stated so a future edit that collapses
	// the two branches fails here rather than in a conclusion.
	for _, want := range []string{"NOT RECORDED", "Do not read this as an empty map"} {
		if !containsStr(fieldAtAbsentFeedNote, want) {
			t.Errorf("the absent-feed note has lost %q", want)
		}
	}
}

func containsStr(hay, needle string) bool {
	return len(hay) >= len(needle) && (hay == needle || indexOf(hay, needle) >= 0)
}

func indexOf(hay, needle string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// Only a finished game whose stream has stopped arriving may be cached.
//
// "The game is over" and "the data is all here" are not the same moment: the
// shipper moves sealed segments every few seconds, so the last of them land
// after the archive row is already written. Caching in that window would freeze
// a reading of half a game and serve it forever.
func TestSettleRequiresAQuietStream(t *testing.T) {
	// No stream at all: the archive is written at game end, so the replay facts
	// are as final as they will get and there is nothing to wait for.
	iv := &investigator{ch: nil}
	if !iv.settled(t.Context(), "") {
		t.Error("a game with no stream should count as settled")
	}
	if !iv.settled(t.Context(), "some-session") {
		t.Error("no ClickHouse client means nothing to wait on")
	}

	// The window has to be longer than a ship cycle, or a settled game is one
	// that merely paused between segments.
	if settleWindow < 30 {
		t.Errorf("settleWindow = %ds, too short to outlast a ship cycle", settleWindow)
	}
}
