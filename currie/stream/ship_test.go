package stream

import (
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nstehr/vimy/vimy-core/wal"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeCH stands in for ClickHouse over HTTP. It records what it was asked and
// lets a test decide what the ledger already contains.
type fakeCH struct {
	mu      sync.Mutex
	queries []string
	bodies  []string
	tokens  []string
	ledger  string // TabSeparated tokens returned by the ledger SELECT
	fail    func(query string) bool
}

func (f *fakeCH) handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("query")
	body, _ := io.ReadAll(r.Body)

	f.mu.Lock()
	f.queries = append(f.queries, q)
	f.bodies = append(f.bodies, string(body))
	if tok := r.URL.Query().Get("insert_deduplication_token"); tok != "" {
		f.tokens = append(f.tokens, tok)
	}
	f.mu.Unlock()

	if f.fail != nil && f.fail(q) {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	if strings.Contains(q, "SELECT concat(session_id") {
		io.WriteString(w, f.ledger)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func newFake(t *testing.T, f *fakeCH) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(s.Close)
	return s
}

func session(t *testing.T, root, id string, segments ...string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, body := range segments {
		write(t, filepath.Join(dir, wal.SegmentName(wal.EvalsPrefix, i+1)), body)
	}
	if err := wal.WriteSession(dir, wal.Session{ID: id, GameID: 147}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Every data INSERT must carry the segment's identity as the deduplication
// token. That token is the only thing standing between an at-least-once
// shipper and doubled rows, and doubled rows are exactly the failure that is
// invisible in the output.
func TestShipTagsEverySegmentWithItsDeduplicationToken(t *testing.T) {
	root := t.TempDir()
	session(t, root, "s1", `{"tick":1,"rule":"a"}`+"\n", `{"tick":2,"rule":"b"}`+"\n")

	f := &fakeCH{}
	srv := newFake(t, f)
	sh := New(root, srv.URL, "currie", "u", "p")

	if _, err := sh.Pass(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"s1/" + wal.SegmentName(wal.EvalsPrefix, 1), "s1/" + wal.SegmentName(wal.EvalsPrefix, 2)}
	if len(f.tokens) != len(want) {
		t.Fatalf("tokens %v, want %v", f.tokens, want)
	}
	for i := range want {
		if f.tokens[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, f.tokens[i], want[i])
		}
	}
}

// A segment already in the ledger must not be sent again. Deduplication makes
// a resend harmless, not free: it still reads and uploads the file.
func TestShipSkipsWhatTheLedgerAlreadyHas(t *testing.T) {
	root := t.TempDir()
	session(t, root, "s1", `{"tick":1}`+"\n", `{"tick":2}`+"\n")

	f := &fakeCH{ledger: "s1/" + wal.SegmentName(wal.EvalsPrefix, 1) + "\n"}
	srv := newFake(t, f)

	st, err := New(root, srv.URL, "currie", "u", "p").Pass(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Segments != 1 {
		t.Fatalf("shipped %d segments, want 1 (the other is in the ledger)", st.Segments)
	}
	if len(f.tokens) != 1 || f.tokens[0] != "s1/"+wal.SegmentName(wal.EvalsPrefix, 2) {
		t.Errorf("sent %v, want only the unshipped segment", f.tokens)
	}
}

// The file the sidecar is still appending to is not shippable at any point.
func TestShipNeverSendsAPartialSegment(t *testing.T) {
	root := t.TempDir()
	dir := session(t, root, "s1", `{"tick":1}`+"\n")
	write(t, filepath.Join(dir, wal.SegmentName(wal.EvalsPrefix, 2)+wal.PartialSuffix), `{"tick":2}`+"\n")

	f := &fakeCH{}
	srv := newFake(t, f)
	st, err := New(root, srv.URL, "currie", "u", "p").Pass(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Segments != 1 {
		t.Fatalf("shipped %d, want 1", st.Segments)
	}
	for _, b := range f.bodies {
		if strings.Contains(b, `"tick":2`) {
			t.Fatal("shipped the partial segment's rows")
		}
	}
}

// If the data lands and the ledger write fails, the pass must fail loudly. The
// segment stays unshipped as far as the ledger is concerned, the next pass
// resends it, and the token makes that resend a no-op.
func TestShipFailsWhenTheLedgerWriteFails(t *testing.T) {
	root := t.TempDir()
	session(t, root, "s1", `{"tick":1}`+"\n")

	f := &fakeCH{fail: func(q string) bool { return strings.Contains(q, "INSERT INTO stream_segments") }}
	srv := newFake(t, f)

	if _, err := New(root, srv.URL, "currie", "u", "p").Pass(context.Background()); err == nil {
		t.Fatal("want an error when the ledger write fails")
	}
}

// game_id is unknown while the game runs and arrives later, so the session row
// has to be rewritten on every pass rather than written once on discovery.
func TestShipRewritesTheSessionRowEveryPass(t *testing.T) {
	root := t.TempDir()
	session(t, root, "s1", `{"tick":1}`+"\n")

	f := &fakeCH{}
	srv := newFake(t, f)
	sh := New(root, srv.URL, "currie", "u", "p")
	ctx := context.Background()
	if _, err := sh.Pass(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := sh.Pass(ctx); err != nil { // nothing new to ship
		t.Fatal(err)
	}
	var n int
	for _, q := range f.queries {
		if strings.Contains(q, "INSERT INTO stream_sessions") {
			n++
		}
	}
	if n != 2 {
		t.Errorf("wrote the session row %d times, want 2 (once per pass)", n)
	}
}

// After a segment lands in the table and the ledger, the local copy is a
// spare. It gets compressed, not deleted: ClickHouse is a local Docker volume
// with no backup, and these files are the only thing a lost volume could be
// rebuilt from.
func TestShipCompressesASegmentOnceItHasLanded(t *testing.T) {
	root := t.TempDir()
	dir := session(t, root, "s1", `{"tick":1}`+"\n")
	plain := filepath.Join(dir, wal.SegmentName(wal.EvalsPrefix, 1))

	f := &fakeCH{}
	srv := newFake(t, f)
	if _, err := New(root, srv.URL, "currie", "u", "p").Pass(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(plain); !os.IsNotExist(err) {
		t.Error("the uncompressed segment is still there")
	}
	gzPath := plain + wal.CompressedExt
	if _, err := os.Stat(gzPath); err != nil {
		t.Fatalf("no compressed segment: %v", err)
	}
	// And it must still be readable as the rows that were sent.
	fh, err := os.Open(gzPath)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	zr, err := gzip.NewReader(fh)
	if err != nil {
		t.Fatalf("not valid gzip: %v", err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != `{"tick":1}`+"\n" {
		t.Errorf("round trip lost data: %q", got)
	}
}

// A compressed segment already in the ledger must not be resent, and if it IS
// resent it must decompress on the way out rather than posting gzip bytes as
// JSON.
func TestShipReadsACompressedSegmentBack(t *testing.T) {
	root := t.TempDir()
	dir := session(t, root, "s1", `{"tick":7}`+"\n")
	plain := filepath.Join(dir, wal.SegmentName(wal.EvalsPrefix, 1))

	// Ship once: lands and compresses.
	f := &fakeCH{}
	srv := newFake(t, f)
	sh := New(root, srv.URL, "currie", "u", "p")
	if _, err := sh.Pass(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plain + wal.CompressedExt); err != nil {
		t.Fatalf("expected a compressed segment: %v", err)
	}

	// Now simulate the ledger having been lost, as after a crash between the
	// data insert and the ledger write.
	f2 := &fakeCH{}
	srv2 := newFake(t, f2)
	if _, err := New(root, srv2.URL, "currie", "u", "p").Pass(context.Background()); err != nil {
		t.Fatal(err)
	}
	var sent string
	for _, b := range f2.bodies {
		if strings.Contains(b, "tick") {
			sent = b
		}
	}
	if sent != `{"tick":7}`+"\n" {
		t.Errorf("resent body = %q, want the decompressed rows", sent)
	}
	if len(f2.tokens) != 1 || f2.tokens[0] != "s1/"+wal.SegmentName(wal.EvalsPrefix, 1) {
		t.Errorf("tokens = %v, want the original logical name so the resend deduplicates", f2.tokens)
	}
}

// With an interval, a failing first pass must not kill the shipper.
//
// It used to. That was harmless when shipping was a process someone started by
// hand and watched exit, and became a real hazard once the server ships in the
// background: a ClickHouse that is a few seconds behind the server at startup
// is the ordinary case, and a shipper that gave up there would leave the whole
// game unshipped with one log line to say so.
func TestRunKeepsShippingAfterAFailedFirstPass(t *testing.T) {
	root := t.TempDir()
	session(t, root, "s1", `{"tick":1,"rule":"a"}`+"\n")

	var passes atomic.Int64
	f := &fakeCH{fail: func(q string) bool {
		// Fail every query of the first pass, then let the shipper through.
		if strings.Contains(q, "SELECT concat(session_id") {
			return passes.Add(1) == 1
		}
		return false
	}}
	srv := newFake(t, f)
	sh := New(root, srv.URL, "currie", "u", "p")
	sh.Log = slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- sh.Run(ctx, 10*time.Millisecond) }()

	// The segment lands on a later pass, not the first.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.tokens)
		f.mu.Unlock()
		if n > 0 {
			cancel()
			if err := <-done; err != nil {
				t.Fatalf("Run returned %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the shipper gave up after the first pass failed")
}

// Without an interval it is a job whose exit code means something, so the
// error is still returned.
func TestRunOnePassStillReportsFailure(t *testing.T) {
	root := t.TempDir()
	session(t, root, "s1", `{"tick":1,"rule":"a"}`+"\n")

	f := &fakeCH{fail: func(q string) bool { return true }}
	srv := newFake(t, f)
	sh := New(root, srv.URL, "currie", "u", "p")

	if err := sh.Run(context.Background(), 0); err == nil {
		t.Fatal("a single pass that failed must report it")
	}
}
