package stream

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nstehr/vimy/vimy-core/wal"
)

// Shipper moves sealed segments into ClickHouse and records what it moved.
//
// At-least-once by construction: the data INSERT and the ledger INSERT cannot
// be one transaction, so a crash between them leaves a segment that is in the
// table and not in the ledger, and the next pass ships it again. The fix is
// not a distributed transaction -- it is insert_deduplication_token, set to
// the segment's path. ClickHouse hashes the inserted block against the tokens
// it has seen and drops the repeat, so the retry is free and the ledger can
// stay an ordinary table that exists for humans to read.
type Shipper struct {
	StreamDir string
	Endpoint  string // e.g. http://localhost:8123
	Database  string
	User      string
	Password  string
	Client    *http.Client
	Log       *slog.Logger
}

// New builds a shipper with the defaults Currie runs with.
func New(streamDir, endpoint, database, user, password string) *Shipper {
	return &Shipper{
		StreamDir: streamDir,
		Endpoint:  strings.TrimRight(endpoint, "/"),
		Database:  database,
		User:      user,
		Password:  password,
		// No timeout on the client: a segment can be tens of megabytes and the
		// per-request context carries the deadline instead.
		Client: &http.Client{},
		Log:    slog.Default(),
	}
}

// Stats is what one pass moved.
type Stats struct {
	Sessions int
	Segments int
	Bytes    int64
	// Compressed counts segments that were already in the table and have now
	// been archived in place.
	Compressed int
}

// Run ships everything ready, then keeps shipping on an interval until the
// context is cancelled. A zero interval makes it a single pass.
//
// With an interval, NO pass is fatal -- including the first. An interval means
// "keep shipping", and a database that is not up yet at the moment the server
// starts is the ordinary case now that Currie ships in the background: dying
// there would leave the whole game unshipped and nothing but one log line to
// say so. Without an interval this is a job whose exit code means something,
// so the error is returned.
func (s *Shipper) Run(ctx context.Context, interval time.Duration) error {
	st, err := s.Pass(ctx)
	switch {
	case err != nil && interval <= 0:
		return err
	case err != nil:
		s.Log.Error("ship pass failed", "error", err)
	default:
		s.logPass(st)
	}
	if interval <= 0 {
		return nil
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			st, err := s.Pass(ctx)
			if err != nil {
				// A pass that fails is not fatal: the segment stays on disk,
				// unshipped, and the next tick tries again. Losing the shipper
				// to a restarting database would lose the game's tail.
				s.Log.Error("ship pass failed", "error", err)
				continue
			}
			s.logPass(st)
		}
	}
}

func (s *Shipper) logPass(st Stats) {
	if st.Segments == 0 && st.Compressed == 0 {
		return
	}
	s.Log.Info("shipped", "segments", st.Segments, "sessions", st.Sessions,
		"bytes", st.Bytes, "compressed", st.Compressed)
}

// Pass ships every sealed segment that the ledger has not already recorded.
func (s *Shipper) Pass(ctx context.Context) (Stats, error) {
	var st Stats

	done, err := s.shipped(ctx)
	if err != nil {
		return st, fmt.Errorf("read ledger: %w", err)
	}

	dirs, err := wal.Sessions(s.StreamDir)
	if err != nil {
		return st, err
	}
	for _, dir := range dirs {
		sess, err := wal.ReadSession(dir)
		if err != nil {
			return st, err
		}
		segs, err := wal.SealedSegments(dir)
		if err != nil {
			return st, err
		}
		// The session row is rewritten every pass, not just on discovery: it
		// carries GameID, which is unknown until the game ends and the
		// retrospective archives it. ReplacingMergeTree collapses the repeats.
		if err := s.upsertSession(ctx, sess); err != nil {
			return st, fmt.Errorf("session %s: %w", sess.ID, err)
		}

		var moved int
		for _, seg := range segs {
			if _, ok := done[seg.Token()]; ok {
				// Already in the table. Compressing here rather than only on
				// the shipping path makes the property "a landed segment is
				// compressed" eventually true: it picks up the archive that
				// predates this, and retries a compression that failed once
				// and was logged rather than raised.
				if !seg.Compressed {
					if err := compressInPlace(seg.Path); err != nil {
						s.Log.Warn("could not compress a landed segment",
							"segment", seg.Token(), "error", err)
					} else {
						st.Compressed++
					}
				}
				continue
			}
			n, err := s.ship(ctx, sess, seg)
			if err != nil {
				return st, fmt.Errorf("ship %s: %w", seg.Token(), err)
			}
			moved++
			st.Segments++
			st.Bytes += seg.Size
			s.Log.Debug("segment shipped", "segment", seg.Token(), "rows", n)
		}
		if moved > 0 {
			st.Sessions++
		}
	}
	return st, nil
}

// ship sends one segment, then records it, then compresses it. The order
// matters: a segment in the ledger but not in the table would be lost
// silently, which is the failure we cannot detect later. The reverse is
// detectable and free to fix.
func (s *Shipper) ship(ctx context.Context, sess wal.Session, seg wal.Segment) (int64, error) {
	f, err := os.Open(seg.Path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var body io.Reader = f
	if seg.Compressed {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", seg.Path, err)
		}
		defer gz.Close()
		body = gz
	}

	spec, ok := streams[seg.Stream()]
	if !ok {
		return 0, fmt.Errorf("no table for stream %q", seg.Stream())
	}

	q := url.Values{}
	q.Set("query", spec.insert())
	q.Set("param_session", sess.ID)
	// Exactly-once, given the table's deduplication window. See the type doc.
	q.Set("insert_deduplication_token", seg.Token())

	if _, err := s.do(ctx, q, body); err != nil {
		return 0, err
	}

	rows := countLines(seg)
	ledger := fmt.Sprintf(
		`INSERT INTO stream_segments (session_id, segment, rows, bytes, ingested_at) VALUES (%s, %s, %d, %d, now())`,
		quote(sess.ID), quote(seg.Name), rows, seg.Size)
	lq := url.Values{}
	lq.Set("query", ledger)
	if _, err := s.do(ctx, lq, nil); err != nil {
		return rows, fmt.Errorf("ledger: %w", err)
	}

	// Compress only now: the rows are in the table and in the ledger, so the
	// local copy is a spare. It stays a spare rather than being deleted --
	// ClickHouse lives in a local Docker volume with no backup, and these
	// files are the only thing a lost volume could be rebuilt from. At ~10:1
	// that costs almost nothing.
	if !seg.Compressed {
		if err := compressInPlace(seg.Path); err != nil {
			// Never fail a shipment over housekeeping: the data landed.
			s.Log.Warn("could not compress a shipped segment", "segment", seg.Token(), "error", err)
		}
	}
	return rows, nil
}

// compressInPlace gzips a sealed, shipped segment and replaces it.
//
// Written to a temporary file and renamed, so an interrupted run leaves either
// the original or the finished archive and never a truncated one. A stray
// .tmp is ignored by SealedSegments, which only matches the segment pattern.
func compressInPlace(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := path + wal.CompressedExt + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err := io.Copy(zw, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := zw.Close(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path+wal.CompressedExt); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Remove(path)
}

// streamSpec says where a stream's segments land and how ClickHouse should
// read them. input() types the incoming JSON and lets the constant session id
// be prepended without the shipper rewriting a single line of the file.
type streamSpec struct {
	table   string
	columns string // the SELECT list, after the session id
	input   string // input()'s column declaration
}

func (s streamSpec) insert() string {
	return "INSERT INTO " + s.table +
		"\nSELECT {session:String}, " + s.columns +
		"\nFROM input('" + s.input + "')\nFORMAT JSONEachRow"
}

// One entry per stream in the WAL. A new stream is a new row here and a new
// table; nothing else in the shipper needs to know about it.
var streams = map[string]streamSpec{
	wal.EvalsPrefix: {
		table:   "stream_rule_evals",
		columns: "tick, rule, rule_set, state, fired, skipped",
		input:   "tick UInt32, rule String, rule_set String, state Int32, fired Bool, skipped Bool",
	},
	wal.EventsPrefix: {
		table:   "stream_events",
		columns: "tick, kind, squad, reason, members, idle, near, spread, attrs",
		input:   "tick UInt32, kind String, squad String, reason String, members Int32, idle Int32, near Int32, spread Int32, attrs Map(String, Float64)",
	},
}

func (s *Shipper) upsertSession(ctx context.Context, sess wal.Session) error {
	started := sess.StartedAt
	if started.IsZero() {
		started = time.Unix(0, 0).UTC()
	}
	q := url.Values{}
	q.Set("query", fmt.Sprintf(
		`INSERT INTO stream_sessions (session_id, started_at, game_id, rules_digest, revision, modified, rows_written, rows_dropped) `+
			`VALUES (%s, toDateTime(%d), %d, %s, %s, %d, %d, %d)`,
		quote(sess.ID), started.Unix(), sess.GameID,
		quote(sess.RulesDigest), quote(sess.Revision), b2i(sess.Modified),
		sess.RowsWritten, sess.RowsDropped))
	_, err := s.do(ctx, q, nil)
	return err
}

// shipped reads the ledger into a set of segment tokens.
func (s *Shipper) shipped(ctx context.Context) (map[string]struct{}, error) {
	q := url.Values{}
	q.Set("query", `SELECT concat(session_id, '/', segment) FROM stream_segments GROUP BY 1 FORMAT TabSeparated`)
	out, err := s.do(ctx, q, nil)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{})
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			set[line] = struct{}{}
		}
	}
	return set, nil
}

// Ping checks the endpoint and that the stream tables exist, so a typo in the
// address fails at startup rather than at the end of the first game.
func (s *Shipper) Ping(ctx context.Context) error {
	q := url.Values{}
	q.Set("query", `SELECT count() FROM stream_segments`)
	_, err := s.do(ctx, q, nil)
	return err
}

func (s *Shipper) do(ctx context.Context, q url.Values, body io.Reader) ([]byte, error) {
	if s.Database != "" {
		q.Set("database", s.Database)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Endpoint+"/?"+q.Encode(), body)
	if err != nil {
		return nil, err
	}
	if s.User != "" {
		req.SetBasicAuth(s.User, s.Password)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, readErr := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clickhouse %s: %s", resp.Status, strings.TrimSpace(string(out)))
	}
	return out, readErr
}

// countLines is for the ledger's row count only. A wrong count there is a
// cosmetic problem, so a read failure reports zero rather than failing a
// shipment that has already landed.
func countLines(seg wal.Segment) int64 {
	f, err := os.Open(seg.Path)
	if err != nil {
		return 0
	}
	defer f.Close()

	var r io.Reader = f
	if seg.Compressed {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return 0
		}
		defer gz.Close()
		r = gz
	}
	var n int64
	buf := make([]byte, 64*1024)
	for {
		c, err := r.Read(buf)
		n += int64(bytes.Count(buf[:c], []byte{'\n'}))
		if err != nil {
			return n
		}
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// quote renders a ClickHouse string literal. Session ids and segment names are
// generated, not user input, but a literal builder that is only correct for
// well-behaved input is the kind that stops being correct quietly.
func quote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}
