package wal

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SegmentWriter appends rows to a stream and seals segments. It is the writer
// half of the contract in wal.go, kept beside it so a writer and a shipper
// cannot disagree about how a segment is named or sealed.
//
// Not safe for concurrent use; Log owns one per stream and serialises through
// its own goroutine.
type SegmentWriter struct {
	dir     string
	prefix  string
	perSeg  int
	maxAge  time.Duration
	seq     int
	n       int
	opened  time.Time
	f       *os.File
	bw      *bufio.Writer
	enc     *json.Encoder
	partial string
}

// NewSegmentWriter seals a segment every rowsPerSegment rows, and -- if maxAge
// is non-zero -- after that long regardless. The age bound is what makes a
// live game visible: without it the tail of a slow stream sits unsealed and
// unshippable until the game ends.
func NewSegmentWriter(dir, prefix string, rowsPerSegment int, maxAge time.Duration) *SegmentWriter {
	if rowsPerSegment <= 0 {
		rowsPerSegment = 50_000
	}
	return &SegmentWriter{dir: dir, prefix: prefix, perSeg: rowsPerSegment, maxAge: maxAge}
}

// Write appends one row, sealing the segment if it is now full.
func (w *SegmentWriter) Write(v any) error {
	if w.f == nil {
		if err := w.open(); err != nil {
			return err
		}
	}
	if err := w.enc.Encode(v); err != nil {
		return err
	}
	w.n++
	if w.n >= w.perSeg {
		return w.Seal()
	}
	return nil
}

// Tick seals an aged-out segment. Called on a timer by Log, so a game that is
// still running has its rows shipped while they are still interesting.
func (w *SegmentWriter) Tick(now time.Time) error {
	if w.f == nil || w.maxAge <= 0 || now.Sub(w.opened) < w.maxAge {
		return nil
	}
	return w.Seal()
}

func (w *SegmentWriter) open() error {
	w.seq++
	w.partial = filepath.Join(w.dir, SegmentName(w.prefix, w.seq)+PartialSuffix)
	f, err := os.Create(w.partial)
	if err != nil {
		return err
	}
	w.f, w.bw = f, bufio.NewWriterSize(f, 256*1024)
	w.enc = json.NewEncoder(w.bw)
	w.n, w.opened = 0, time.Now()
	return nil
}

// Seal flushes and renames. The rename is the commit: until it happens no
// reader can see the file, and after it happens the file never changes again.
func (w *SegmentWriter) Seal() error {
	if w.f == nil {
		return nil
	}
	if err := w.bw.Flush(); err != nil {
		return err
	}
	if err := w.f.Close(); err != nil {
		return err
	}
	w.f, w.bw, w.enc = nil, nil, nil
	if w.n == 0 {
		return os.Remove(w.partial)
	}
	return os.Rename(w.partial, strings.TrimSuffix(w.partial, PartialSuffix))
}

// Close seals whatever is open, including a short final segment. Without it
// the tail of every game is lost.
func (w *SegmentWriter) Close() error { return w.Seal() }

// Log is the sidecar's end of the WAL: two streams, one background writer, and
// no blocking.
//
// The rule loop must never wait on this. Rows go to a buffered channel and are
// DROPPED when it is full, because a stalled game is a worse outcome than an
// incomplete log. Drops are counted and written into session.json, so an
// analysis can see that it is looking at a hole -- a silent drop counter would
// rebuild exactly the class of quietly-wrong number this telemetry exists to
// kill.
type Log struct {
	dir     string
	rows    chan entry
	done    chan struct{}
	dropped atomic.Uint64
	written atomic.Uint64

	mu      sync.Mutex
	session Session
	closed  bool
}

type entry struct {
	stream string
	v      any
}

// LogOptions tunes the segmenting. The defaults suit a live game.
type LogOptions struct {
	Buffer         int           // channel depth; 0 means 16384
	RowsPerSegment int           // 0 means 50000
	MaxAge         time.Duration // 0 means 10s
}

// Open starts a session under dir/<session.ID> and returns its log.
func Open(dir string, s Session, opt LogOptions) (*Log, error) {
	if opt.Buffer <= 0 {
		opt.Buffer = 16384
	}
	if opt.RowsPerSegment <= 0 {
		opt.RowsPerSegment = 50_000
	}
	if opt.MaxAge <= 0 {
		opt.MaxAge = 10 * time.Second
	}
	sdir := filepath.Join(dir, s.ID)
	if err := os.MkdirAll(sdir, 0o755); err != nil {
		return nil, err
	}
	if s.StartedAt.IsZero() {
		s.StartedAt = time.Now().UTC()
	}
	if err := WriteSession(sdir, s); err != nil {
		return nil, err
	}

	l := &Log{
		dir:     sdir,
		rows:    make(chan entry, opt.Buffer),
		done:    make(chan struct{}),
		session: s,
	}
	go l.run(opt)
	return l, nil
}

func (l *Log) run(opt LogOptions) {
	defer close(l.done)
	writers := map[string]*SegmentWriter{
		EvalsPrefix:  NewSegmentWriter(l.dir, EvalsPrefix, opt.RowsPerSegment, opt.MaxAge),
		EventsPrefix: NewSegmentWriter(l.dir, EventsPrefix, opt.RowsPerSegment, opt.MaxAge),
		UnitsPrefix:  NewSegmentWriter(l.dir, UnitsPrefix, opt.RowsPerSegment, opt.MaxAge),
	}
	defer func() {
		for _, w := range writers {
			if err := w.Close(); err != nil {
				slog.Error("wal: sealing the final segment failed", "error", err)
			}
		}
	}()

	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case e, ok := <-l.rows:
			if !ok {
				return
			}
			w := writers[e.stream]
			if w == nil {
				continue
			}
			if err := w.Write(e.v); err != nil {
				// One bad write must not take the game down, and must not be
				// silent either: it is a hole in the data.
				l.dropped.Add(1)
				slog.Error("wal: write failed", "stream", e.stream, "error", err)
				continue
			}
			l.written.Add(1)
		case now := <-t.C:
			for _, w := range writers {
				if err := w.Tick(now); err != nil {
					slog.Error("wal: sealing an aged segment failed", "error", err)
				}
			}
		}
	}
}

// WriteRow queues one rule evaluation. Never blocks.
func (l *Log) WriteRow(r Row) { l.offer(EvalsPrefix, r) }

// WriteUnit queues one unit sample. Never blocks, and drops under pressure
// like every other stream: a missing frame is a gap in a replay, not a fault.
func (l *Log) WriteUnit(u Unit) { l.offer(UnitsPrefix, u) }

// WriteEvent queues one event. Never blocks.
func (l *Log) WriteEvent(e Event) { l.offer(EventsPrefix, e) }

func (l *Log) offer(stream string, v any) {
	if l == nil {
		return
	}
	select {
	case l.rows <- entry{stream: stream, v: v}:
	default:
		l.dropped.Add(1)
	}
}

// Stats reports what the log has taken and what it has thrown away.
func (l *Log) Stats() (written, dropped uint64) {
	if l == nil {
		return 0, 0
	}
	return l.written.Load(), l.dropped.Load()
}

// Finish stamps what is only knowable at the end -- the archive id the game
// became, and how much was dropped -- then drains and seals.
//
// Safe to call twice; the second call is a no-op.
func (l *Log) Finish(gameID int64) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	l.mu.Unlock()

	close(l.rows)
	<-l.done

	written, dropped := l.Stats()
	s := l.session
	s.GameID = gameID
	s.RowsWritten = written
	s.RowsDropped = dropped
	if dropped > 0 {
		slog.Warn("wal: rows were dropped; the log has holes",
			"dropped", dropped, "written", written, "session", s.ID)
	}
	return WriteSession(l.dir, s)
}
