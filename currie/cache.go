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
// An archived game never changes, so a reading of it holds forever — but only
// against the rules it was read from. Editing a `.vy` file changes the replay,
// changes every number the model saw, and makes its prose wrong. So the key is
// the game plus a fingerprint of the sources: edited rules simply miss, rather
// than serving a stale answer as current.
//
// Only the model's output is cached. A replay costs half a second and a sweep a
// few, cheaper than reasoning about their staleness; a reading costs thirty
// seconds and a paid call.
//
// Its own database, not a table in the archive. `vimy.db` runs journal_mode=
// delete with no busy timeout, so writing there would take a lock the sidecar
// needs — and this is derived data that should be droppable without alarm.
type cache struct {
	db      *sql.DB
	queries *db.Queries
	rules   string // fingerprint of the .vy sources
}

// Applied rather than migrated: a cache rebuildable from its inputs has no
// history worth preserving, so a schema change is a DROP. Hence a plain
// schema.sql here where vimy-core has a migrations directory.
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
	// WAL and a busy timeout — the lock problem above, not repeated here.
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

// analysisVersion invalidates readings when what the model is shown changes, as
// opposed to what the game did. The source fingerprint catches a rule edit but
// not a change to the analysis itself. Bump it on any change to Insight's input.
const analysisVersion = 2

// fingerprint hashes the rule sources, sorted so listing order can't change the
// answer.
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
		// A corrupt row costs one regenerated reading; not worth surfacing.
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

// Readings is every stored reading for the current rules, newest first. No page
// uses it yet — it is what the cross-game work will read, and the reason this is
// a table rather than a directory of files.
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
