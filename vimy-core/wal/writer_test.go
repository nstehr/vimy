package wal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The rule loop must never wait on telemetry. A full buffer drops rows; it does
// not block. If this ever regresses, a slow disk stalls the game.
func TestLogDropsRatherThanBlock(t *testing.T) {
	l, err := Open(t.TempDir(), Session{ID: "s"}, LogOptions{Buffer: 1, RowsPerSegment: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	// Far more than the buffer, with the writer goroutine unable to keep up.
	for i := 0; i < 100_000; i++ {
		l.WriteEvent(Event{Kind: "rally", Spread: i})
	}
	if err := l.Finish(1); err != nil {
		t.Fatal(err)
	}
	written, dropped := l.Stats()
	if written+dropped != 100_000 {
		t.Errorf("written %d + dropped %d = %d, want every row accounted for",
			written, dropped, written+dropped)
	}
}

// A drop that nobody can see is the failure this telemetry exists to kill, so
// the count has to survive into session.json where an analysis can find it.
func TestFinishRecordsDropsAndTheArchiveID(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, Session{ID: "s", RulesDigest: "abc123"}, LogOptions{Buffer: 1})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50_000; i++ {
		l.WriteEvent(Event{Kind: "rally"})
	}
	if err := l.Finish(147); err != nil {
		t.Fatal(err)
	}

	got, err := ReadSession(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	if got.GameID != 147 {
		t.Errorf("game_id = %d, want 147", got.GameID)
	}
	if got.RulesDigest != "abc123" {
		t.Errorf("rules_digest = %q, want it preserved across Finish", got.RulesDigest)
	}
	if got.RowsWritten+got.RowsDropped != 50_000 {
		t.Errorf("written %d + dropped %d, want 50000 accounted for",
			got.RowsWritten, got.RowsDropped)
	}
}

// Finish is called from two places -- the retrospective, and a defer guarding
// the case where it never gets that far. The second must not undo the first.
func TestFinishIsIdempotentAndKeepsTheFirstArchiveID(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, Session{ID: "s"}, LogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.WriteEvent(Event{Kind: "rally"})
	if err := l.Finish(147); err != nil {
		t.Fatal(err)
	}
	if err := l.Finish(0); err != nil { // the deferred guard, running after
		t.Fatal(err)
	}
	got, err := ReadSession(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	if got.GameID != 147 {
		t.Errorf("game_id = %d, want the first Finish to win", got.GameID)
	}
}

// The two streams must land in different files, or the shipper cannot tell
// which table a segment belongs in.
func TestLogKeepsTheStreamsApart(t *testing.T) {
	dir := t.TempDir()
	l, err := Open(dir, Session{ID: "s"}, LogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.WriteRow(Row{Rule: "build-power"})
	l.WriteEvent(Event{Kind: "rally", Members: 4})
	if err := l.Finish(1); err != nil {
		t.Fatal(err)
	}

	segs, err := SealedSegments(filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, s := range segs {
		b, err := os.ReadFile(s.Path)
		if err != nil {
			t.Fatal(err)
		}
		seen[s.Stream()] = string(b)
	}
	if !strings.Contains(seen[EvalsPrefix], "build-power") {
		t.Errorf("the evaluation did not land in the evals stream: %q", seen[EvalsPrefix])
	}
	if !strings.Contains(seen[EventsPrefix], `"kind":"rally"`) {
		t.Errorf("the event did not land in the events stream: %q", seen[EventsPrefix])
	}
}

// ClickHouse reads these with input(), which is positional about nothing and
// strict about types. A renamed or retyped field breaks ingestion silently at
// the far end, so pin the wire shape here where it is cheap to see.
func TestEventWireShape(t *testing.T) {
	b, err := json.Marshal(Event{
		Tick: 9, Kind: "rally", Squad: "ground-attack",
		Members: 4, Idle: 2, Spread: 24,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"tick":9,"kind":"rally","squad":"ground-attack","members":4,"idle":2,"near":0,"spread":24}`
	if string(b) != want {
		t.Errorf("wire shape changed:\n got %s\nwant %s", b, want)
	}

	// A transit row carries the continuous distance the in-memory sampler
	// throws away. Attrs is a Map(String, Float64) at the far end, so a key
	// renamed here silently becomes a zero there.
	b, err = json.Marshal(Event{
		Tick: 9, Kind: "transit", Squad: "ground-attack",
		Members: 5, Near: 3, Spread: 19,
		Attrs: map[string]float64{"target_fraction": 0.31},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = `{"tick":9,"kind":"transit","squad":"ground-attack","members":5,"idle":0,"near":3,"spread":19,"attrs":{"target_fraction":0.31}}`
	if string(b) != want {
		t.Errorf("transit wire shape changed:\n got %s\nwant %s", b, want)
	}
}

// A segment held open until the game ends is a segment nobody can watch. The
// age bound is what makes a live game visible to the shipper.
func TestSegmentsSealOnAge(t *testing.T) {
	dir := t.TempDir()
	w := NewSegmentWriter(dir, EventsPrefix, 1<<30, time.Millisecond)
	if err := w.Write(Event{Kind: "rally"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := w.Tick(time.Now()); err != nil {
		t.Fatal(err)
	}
	segs, err := SealedSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 {
		t.Fatalf("got %d sealed segments, want 1 sealed by age", len(segs))
	}
}
