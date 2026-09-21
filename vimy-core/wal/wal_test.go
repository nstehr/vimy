package wal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A shipper must never read the file the sidecar is appending to. That is the
// whole reason segments are sealed by rename, so it is the first thing to pin.
func TestSealedSegmentsIgnoresThePartialFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, SegmentName(EvalsPrefix, 1)), "{}\n")
	write(t, filepath.Join(dir, SegmentName(EvalsPrefix, 2)+PartialSuffix), "{}\n")

	segs, err := SealedSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 || segs[0].Name != SegmentName(EvalsPrefix, 1) {
		t.Fatalf("got %v, want only %s", names(segs), SegmentName(EvalsPrefix, 1))
	}
}

// Lexical order has to be write order, or a crash mid-game replays segments
// out of sequence. Zero-padding is what makes that true, so check it holds
// across the digit boundary where an unpadded name would sort 10 before 9.
func TestSegmentsShipInWriteOrder(t *testing.T) {
	dir := t.TempDir()
	for _, seq := range []int{10, 2, 1, 9} {
		write(t, filepath.Join(dir, SegmentName(EvalsPrefix, seq)), "{}\n")
	}
	segs, err := SealedSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{SegmentName(EvalsPrefix, 1), SegmentName(EvalsPrefix, 2), SegmentName(EvalsPrefix, 9), SegmentName(EvalsPrefix, 10)}
	got := names(segs)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
}

// A session that died before writing session.json still has rows worth having.
func TestReadSessionSurvivesAMissingSessionFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "abc123")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := ReadSession(dir)
	if err != nil {
		t.Fatalf("ReadSession: %v", err)
	}
	if s.ID != "abc123" {
		t.Errorf("id = %q, want the directory name", s.ID)
	}
	if s.GameID != 0 {
		t.Errorf("game_id = %d, want 0 for a session that never finished", s.GameID)
	}
}

func TestSessionRoundTrips(t *testing.T) {
	dir := t.TempDir()
	want := Session{ID: filepath.Base(dir), StartedAt: time.Now().UTC().Truncate(time.Second), GameID: 147}
	if err := WriteSession(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.GameID != want.GameID || !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// Sealing happens on a row count, and the tail of a game is whatever is left
// over -- so Close must seal the short final segment or the last rows never
// ship. An empty trailing segment must not be sealed at all.
func TestWriterSealsOnCountAndOnClose(t *testing.T) {
	dir := t.TempDir()
	w := NewSegmentWriter(dir, EvalsPrefix, 2, 0)
	for i := 0; i < 5; i++ {
		if err := w.Write(Row{Tick: i, Rule: "r", StateIdx: -1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	segs, err := SealedSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	// 5 rows at 2 per segment: two full, one holding the remainder.
	if len(segs) != 3 {
		t.Fatalf("got %d segments %v, want 3", len(segs), names(segs))
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*"+PartialSuffix)); len(left) != 0 {
		t.Errorf("left %v unsealed", left)
	}
}

func TestWriterLeavesNoEmptySegment(t *testing.T) {
	dir := t.TempDir()
	w := NewSegmentWriter(dir, EvalsPrefix, 2, 0)
	for i := 0; i < 4; i++ { // exactly two full segments, nothing left over
		if err := w.Write(Row{Tick: i, Rule: "r", StateIdx: -1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	segs, err := SealedSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 2 {
		t.Fatalf("got %d segments %v, want 2 with no empty third", len(segs), names(segs))
	}
}

func TestSessionsIgnoresAMissingStreamDir(t *testing.T) {
	got, err := Sessions(filepath.Join(t.TempDir(), "never-created"))
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func names(segs []Segment) []string {
	out := make([]string, len(segs))
	for i, s := range segs {
		out[i] = s.Name
	}
	return out
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A shipped segment is compressed in place. It must keep its logical name, or
// its deduplication token changes and ClickHouse takes the resend as new data
// -- doubling the segment with nothing in the output to say so.
func TestCompressingASegmentKeepsItsIdentity(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	plain := SegmentName(EvalsPrefix, 1)
	write(t, filepath.Join(dir, plain), "{}\n")

	before, err := SealedSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || before[0].Compressed {
		t.Fatalf("got %+v, want one uncompressed segment", before)
	}

	// Stand in for the shipper compressing it.
	if err := os.Rename(filepath.Join(dir, plain), filepath.Join(dir, plain+CompressedExt)); err != nil {
		t.Fatal(err)
	}

	after, err := SealedSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("got %d segments after compression, want 1 -- a .gz segment is still a segment", len(after))
	}
	if !after[0].Compressed {
		t.Error("the segment does not report itself compressed; the shipper would read gzip as JSON")
	}
	if after[0].Token() != before[0].Token() {
		t.Errorf("token changed on compression: %q -> %q; the resend would not deduplicate",
			before[0].Token(), after[0].Token())
	}
	if after[0].Stream() != EvalsPrefix {
		t.Errorf("stream = %q, want %q: a .gz segment must still route to its table",
			after[0].Stream(), EvalsPrefix)
	}
}

// An interrupted compression leaves a .tmp behind. It is not a segment and
// must never be shipped -- it is a partial gzip of data already in the table.
func TestSealedSegmentsIgnoresAnInterruptedCompression(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, SegmentName(EvalsPrefix, 1)), "{}\n")
	write(t, filepath.Join(dir, SegmentName(EvalsPrefix, 1)+CompressedExt+".tmp"), "garbage")

	segs, err := SealedSegments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 1 {
		t.Fatalf("got %d segments %v, want only the real one", len(segs), names(segs))
	}
}
