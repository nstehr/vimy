package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nstehr/vimy/currie/store/db"
	_ "modernc.org/sqlite"
)

// Keeping the model's reading between runs.
//
// An archived game does not change, so its reading is valid forever — but only
// against the rules it was read from. Edit a `.vy` file and the replay changes,
// which changes every number the model was shown, which makes the prose it
// wrote about them wrong. So the key is the game *and* a fingerprint of the
// sources: change the rules and the old answer is simply not found, rather than
// served as though it still applied.
//
// Only the model's output is kept. A replay costs half a second and a sweep a
// few, which is cheaper than being careful about staleness; a reading costs
// thirty seconds and a paid call.
//
// Its own database rather than a table in the archive. Two reasons, and the
// first is not aesthetic: `vimy.db` runs in `journal_mode=delete` with no busy
// timeout, so writing to it from here while a game is being recorded would take
// a lock the sidecar needs. The second is that this is derived data — it can be
// deleted wholesale and rebuilt, and it should not sit in the system of record
// where a `DROP` would be frightening.
type cache struct {
	db      *sql.DB
	queries *db.Queries
	rules   string // fingerprint of the .vy sources
}

// The schema is applied rather than migrated. A cache that can be rebuilt from
// its inputs has no history worth preserving, so a change here is a `DROP` and
// a rebuild rather than a migration to get right — which is why `sqlc` reads a
// plain `schema.sql` here and a migrations directory in vimy-core.
//
//go:embed store/schema.sql
var cacheSchema string

func newCache(stateDir, rulesDir string) (*cache, error) {
	fp, err := fingerprint(rulesDir)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(stateDir, "currie")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cache dir: %w", err)
	}
	// WAL and a busy timeout: the mistake this file exists to avoid is worth
	// not repeating in its own database.
	path := filepath.Join(dir, "currie.db")
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := sqlDB.Exec(cacheSchema); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("cache schema: %w", err)
	}
	return &cache{db: sqlDB, queries: db.New(sqlDB), rules: fp}, nil
}

func (c *cache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

// analysisVersion invalidates stored readings when what the model is SHOWN
// changes, as opposed to what the game did.
//
// The key was the game plus a fingerprint of the .vy sources, which catches a
// rule edit but not a change to the analysis itself. Currie stopped feeding the
// model completion guards on 2026-09-08; without this, every game already read
// would keep serving prose written from the old, noisier input. Bump on any
// change to what goes into Insight.
const analysisVersion = 2

// fingerprint hashes the rule sources, sorted so a directory listing's order
// cannot change the answer.
func fingerprint(rulesDir string) (string, error) {
	paths, err := ruleArgs(rulesDir)
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	h := sha256.New()
	fmt.Fprintf(h, "analysis=%d\x00", analysisVersion)
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("%s: %w", p, err)
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.Base(p), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// read returns a stored reading, or nil when there is none for this game under
// these rules.
func (c *cache) read(game int64) *Insight {
	ctx := context.Background()
	blob, err := c.queries.GetInsight(ctx, db.GetInsightParams{GameID: game, Rules: c.rules})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("cannot read cached insight", "game", game, "error", err)
		}
		return nil
	}
	var ins Insight
	if err := json.Unmarshal([]byte(blob), &ins); err != nil {
		// A corrupt row is not worth reporting to the reader: the cost of
		// missing it is one regenerated reading.
		slog.Warn("discarding unreadable cache entry", "game", game, "error", err)
		_ = c.queries.DropInsight(ctx, db.DropInsightParams{GameID: game, Rules: c.rules})
		return nil
	}
	return &ins
}

func (c *cache) write(game int64, ins *Insight) {
	blob, err := json.Marshal(ins)
	if err != nil {
		slog.Warn("cannot encode reading", "game", game, "error", err)
		return
	}
	if err := c.queries.PutInsight(context.Background(), db.PutInsightParams{
		GameID:      game,
		Rules:       c.rules,
		CreatedAt:   time.Now().Unix(),
		Summary:     ins.Summary,
		Suggestion:  ins.Suggestion,
		InsightJson: string(blob),
	}); err != nil {
		slog.Warn("cannot store reading", "game", game, "error", err)
	}
}

// Readings is every stored reading for the current rules, newest first.
//
// Not used by a page yet; it is what the cross-game work will read, and the
// reason this is a table rather than a directory of files.
func (c *cache) Readings(ctx context.Context) (map[int64]string, error) {
	rows, err := c.queries.ListInsights(ctx, c.rules)
	if err != nil {
		return nil, fmt.Errorf("list readings: %w", err)
	}
	out := make(map[int64]string, len(rows))
	for _, r := range rows {
		out[r.GameID] = r.Summary
	}
	return out, nil
}
