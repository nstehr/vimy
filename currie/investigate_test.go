package main

import (
	"strings"
	"testing"

	"github.com/nstehr/vimy/vimy-core/store"
)

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

// A malformed step must cost a turn, not the investigation.
//
// Game 176 died at step 5 because the model tried to finish and omitted two
// fields it had nothing to say for. BAML rejected all five candidate parses and
// the run was lost along with four good tool results already in the trace.
func TestMalformedStepsAreToleratedButBounded(t *testing.T) {
	if maxMalformedSteps <= 0 || maxMalformedSteps >= maxInvestigationSteps {
		t.Errorf("maxMalformed = %d against a budget of %d: it must absorb a slip without spending the whole run",
			maxMalformedSteps, maxInvestigationSteps)
	}
}

// One investigation of game 180 lost three consecutive steps to the session id:
// `{session}` without a type is a syntax error at the closing brace, and
// `session = '{session}'` is a quoted placeholder that never substitutes against
// a column no table has. Each of those questions was a good one.
func TestQueryRepairsTheSessionFilter(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SELECT tick FROM stream_events WHERE session_id = {session}",
			"SELECT tick FROM stream_events WHERE session_id = {session:String}"},
		{"SELECT tick FROM stream_events WHERE session_id = '{session}'",
			"SELECT tick FROM stream_events WHERE session_id = {session:String}"},
		{"SELECT tick FROM stream_events WHERE session_id = {session_id}",
			"SELECT tick FROM stream_events WHERE session_id = {session:String}"},
		{"SELECT tick FROM stream_events WHERE session_id = { session : String }",
			"SELECT tick FROM stream_events WHERE session_id = {session:String}"},
		// The wrong column, with and without a placeholder beside it.
		{"SELECT tick FROM stream_events WHERE session = '{session}'",
			"SELECT tick FROM stream_events WHERE session_id = {session:String}"},
		{"SELECT tick FROM stream_units WHERE session IN ('a','b')",
			"SELECT tick FROM stream_units WHERE session_id IN ('a','b')"},
		// A literal id is what the menu now asks for, and must pass through.
		{"SELECT tick FROM stream_events WHERE session_id = '20260925-144438-b8814fed'",
			"SELECT tick FROM stream_events WHERE session_id = '20260925-144438-b8814fed'"},
		// Nothing resembling a session filter is left alone, including columns
		// that merely start with the same letters.
		{"SELECT session_idx FROM t WHERE sessions = 1", "SELECT session_idx FROM t WHERE sessions = 1"},
	}
	for _, c := range cases {
		got := wrongSessionColumn.ReplaceAllString(sessionPlaceholder.ReplaceAllString(c.in, "{session:String}"), "session_id$1")
		if got != c.want {
			t.Errorf("\n in   %s\n got  %s\n want %s", c.in, got, c.want)
		}
	}
}

// The model is told the id rather than asked to spell a placeholder for it.
func TestMenuNamesTheSession(t *testing.T) {
	m := menuFor("20260925-144438-b8814fed")
	if !strings.Contains(m, "session_id = '20260925-144438-b8814fed'") {
		t.Error("the menu does not show the literal filter to write")
	}
	if strings.Contains(m, "Pass the session id as") {
		t.Error("the placeholder instruction is still in the menu")
	}
	if !strings.Contains(menuFor(""), "never streamed") {
		t.Error("a game with no telemetry should be told so, not handed an empty filter")
	}
}

// The facts bundle was entirely failure-side: blocked clauses, dead rules,
// gates, missing things. So an investigation of game 180 read squad-attack's 348
// blocks as "the army never launched" while squad-attack-known-base - same
// exclusive category, same ready-ratio clause, higher priority at that
// aggression - had acted 725 times. The model could not see that, and now must.
func TestFactsCarryWhatActuallyFired(t *testing.T) {
	r := &Replay{
		Game: store.ReplayableGame{ID: 180, DurationTicks: 34790, OurFaction: "england"},
		Firings: map[string]store.Firing{
			"squad-attack-known-base": {Matched: 1195, Acted: 725, FirstTick: 4630, LastTick: 30550},
			"produce-infantry":        {Matched: 400, Acted: 112, FirstTick: 1610, LastTick: 30000},
			// Matched but never acted: not a firing rule, and must not be listed
			// as one.
			"capture-building": {Matched: 6, Acted: 0, FirstTick: 100, LastTick: 200},
		},
	}
	f := facts(r)
	if len(f.Top_firing) != 2 {
		t.Fatalf("want the two rules that acted, got %d", len(f.Top_firing))
	}
	if f.Top_firing[0].Name != "squad-attack-known-base" || f.Top_firing[0].Acted != 725 {
		t.Errorf("the busiest rule is not first: %+v", f.Top_firing[0])
	}
	if f.Top_firing[0].Last_tick != 30550 {
		t.Errorf("last_tick dropped: %+v", f.Top_firing[0])
	}
	for _, fr := range f.Top_firing {
		if fr.Name == "capture-building" {
			t.Error("a rule that matched and never acted was reported as having fired")
		}
	}
}

// The menu must warn about the trap the facts now let the model avoid.
func TestMenuWarnsAboutExclusiveCategories(t *testing.T) {
	m := menuFor("s1")
	for _, want := range []string{"EXCLUSIVE", "top_firing", "preempted"} {
		if !strings.Contains(m, want) {
			t.Errorf("the menu never mentions %q", want)
		}
	}
}
