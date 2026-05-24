// Package store persists cross-game memory — win/loss records, archived
// doctrines with retrospective ratings, and lessons learned from prior games —
// in a local SQLite database. The database is created and migrated on demand
// via embedded goose migrations.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/nstehr/vimy/vimy-core/store/db"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the sqlc-generated Queries with domain-level helpers.
type Store struct {
	db      *sql.DB
	queries *db.Queries
}

// GameRecord is the legacy summary shape used by the dashboard.
type GameRecord struct {
	Faction string `json:"faction"`
	Won     bool   `json:"won"`
}

// GameContext carries per-game metadata that accompanies an archived game.
type GameContext struct {
	OurFaction      string
	OpponentFaction string
	MapWidth        int
	MapHeight       int
	DurationTicks   int
	Won             bool
}

// ArchivedDoctrine is one doctrine snapshot from a prior game, ready to be
// injected into the strategist prompt as a few-shot example.
type ArchivedDoctrine struct {
	DoctrineJSON    string
	Rating          string // "strong"|"adequate"|"weak" — empty if unrated
	RatingReason    string
	Won             bool
	QualityTag      string // "exemplary"|"mixed"|"cautionary"
	OpponentFaction string
	PlayedAt        time.Time
}

// LessonRow is a retrieved lesson ready for prompt injection.
type LessonRow struct {
	Trigger    string
	Guidance   string
	Confidence float64
}

// InputDoctrine is the minimal shape the store needs to archive one doctrine.
// Callers pass a slice of these (serialized) rather than the full agent types,
// to keep the store package free of agent/rules imports.
type InputDoctrine struct {
	Tick         int
	DoctrineJSON string // already-serialized rules.Doctrine
	Rating       string // "" if unrated
	RatingReason string

	// Rule-engine trace for this doctrine's window.
	RuleSetJSON  string                 // JSON array of rule names compiled during the window; "" skips storage
	RuleFirings  []InputRuleFiring      // per-rule firing stats; empty means no rule fired (or pre-instrumentation)
}

// InputRuleFiring is a per-rule, per-doctrine-window firing record.
type InputRuleFiring struct {
	RuleName  string
	FireCount int
	FirstTick int
	LastTick  int
}

// InputLesson is a lesson destined for the lessons table.
type InputLesson struct {
	Trigger    string
	Guidance   string
	Confidence float64
}

// MemoryFilter controls retrieval for a new game's opening evaluation.
type MemoryFilter struct {
	OurFaction       string
	OpponentFaction  string // pass "unknown" to disable opponent-faction filtering
	TopKExemplars    int
	TopKCautionaries int
	TopMLessons      int
}

// SameSideFactions returns the factions that share a tech tree with the
// given faction (vimy-wkv). Memory queries should match any faction on the
// same side: france/england/germany are Allied, russia/ukraine are Soviet.
// Cross-side leakage stays blocked because Soviet and Allied units differ
// (APC vs Ranger, Iron Curtain vs Chronosphere, etc.).
func SameSideFactions(faction string) []string {
	switch faction {
	case "france", "england", "germany":
		return []string{"france", "england", "germany"}
	case "russia", "ukraine":
		return []string{"russia", "ukraine"}
	default:
		return []string{faction}
	}
}

// MemoryContext is the assembled retrieval result. Exemplars are examples
// worth emulating; Cautionaries are patterns worth AVOIDING (framed as
// negative constraints, not as templates).
type MemoryContext struct {
	Exemplars    []ArchivedDoctrine
	Cautionaries []ArchivedDoctrine
	Lessons      []LessonRow
}

// New opens (or creates) the SQLite database at dir/vimy.db, applies pending
// migrations, and returns a ready-to-use Store. If dir is empty, defaults to
// ~/.vimy.
func New(dir string) (*Store, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		dir = filepath.Join(home, ".vimy")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	dbPath := filepath.Join(dir, "vimy.db")
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}

	s := &Store{db: sqlDB, queries: db.New(sqlDB)}
	if err := s.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	slog.Info("store ready", "path", dbPath)
	return s, nil
}

// NewFromDB constructs a Store from an already-open *sql.DB. Intended for
// tests that want an in-memory database.
func NewFromDB(sqlDB *sql.DB) (*Store, error) {
	s := &Store{db: sqlDB, queries: db.New(sqlDB)}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.Up(s.db, "migrations")
}

// RecordGame inserts a minimal game row. Used when no retrospective is
// available — preserves the legacy dashboard counters.
func (s *Store) RecordGame(r GameRecord) error {
	_, err := s.queries.InsertGame(context.Background(), db.InsertGameParams{
		PlayedAt:        time.Now().Unix(),
		OurFaction:      r.Faction,
		OpponentFaction: sql.NullString{},
		MapWidth:        0,
		MapHeight:       0,
		DurationTicks:   0,
		Won:             boolToInt(r.Won),
	})
	if err != nil {
		slog.Error("RecordGame failed", "error", err)
	}
	return err
}

// Games returns all recorded games as legacy summaries, ordered by play time.
func (s *Store) Games() []GameRecord {
	rows, err := s.queries.ListGames(context.Background())
	if err != nil {
		slog.Warn("ListGames failed", "error", err)
		return nil
	}
	out := make([]GameRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, GameRecord{Faction: r.OurFaction, Won: r.Won == 1})
	}
	return out
}

// Wins returns the total number of wins across all archived games.
func (s *Store) Wins() int {
	n, err := s.queries.CountWins(context.Background())
	if err != nil {
		slog.Warn("CountWins failed", "error", err)
		return 0
	}
	return int(n)
}

// Losses returns the total number of losses across all archived games.
func (s *Store) Losses() int {
	n, err := s.queries.CountLosses(context.Background())
	if err != nil {
		slog.Warn("CountLosses failed", "error", err)
		return 0
	}
	return int(n)
}

// ArchiveGame inserts the game row and all per-doctrine records. reviewJSON
// is the JSON-encoded GameReview (may be ""); qualityTag may be "" if no
// review was produced. Returns the new game's ID.
func (s *Store) ArchiveGame(
	ctx context.Context,
	gameCtx GameContext,
	qualityTag, reviewJSON string,
	doctrines []InputDoctrine,
) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := s.queries.WithTx(tx)
	gameID, err := q.InsertGame(ctx, db.InsertGameParams{
		PlayedAt:        time.Now().Unix(),
		OurFaction:      gameCtx.OurFaction,
		OpponentFaction: nullableString(gameCtx.OpponentFaction),
		MapWidth:        int64(gameCtx.MapWidth),
		MapHeight:       int64(gameCtx.MapHeight),
		DurationTicks:   int64(gameCtx.DurationTicks),
		Won:             boolToInt(gameCtx.Won),
		QualityTag:      nullableString(qualityTag),
		ReviewJson:      nullableString(reviewJSON),
	})
	if err != nil {
		return 0, fmt.Errorf("insert game: %w", err)
	}

	for _, d := range doctrines {
		doctrineID, err := q.InsertDoctrine(ctx, db.InsertDoctrineParams{
			GameID:       gameID,
			Tick:         int64(d.Tick),
			DoctrineJson: d.DoctrineJSON,
			Rating:       nullableString(d.Rating),
			RatingReason: nullableString(d.RatingReason),
			RuleSetJson:  nullableString(d.RuleSetJSON),
		})
		if err != nil {
			return 0, fmt.Errorf("insert doctrine: %w", err)
		}
		for _, f := range d.RuleFirings {
			if err := q.InsertRuleFiring(ctx, db.InsertRuleFiringParams{
				DoctrineID: doctrineID,
				RuleName:   f.RuleName,
				FireCount:  int64(f.FireCount),
				FirstTick:  int64(f.FirstTick),
				LastTick:   int64(f.LastTick),
			}); err != nil {
				return 0, fmt.Errorf("insert rule_firing: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return gameID, nil
}

// ArchiveLessons inserts a batch of lessons tied to the given game.
func (s *Store) ArchiveLessons(
	ctx context.Context,
	gameID int64,
	ourFaction, opponentFaction string,
	lessons []InputLesson,
) error {
	if len(lessons) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := s.queries.WithTx(tx)
	now := time.Now().Unix()
	for _, l := range lessons {
		if err := q.InsertLesson(ctx, db.InsertLessonParams{
			GameID:         gameID,
			CreatedAt:      now,
			TriggerText:    l.Trigger,
			GuidanceText:   l.Guidance,
			Confidence:     l.Confidence,
			AppliesFaction: ourFaction,
			AppliesVs:      nullableString(opponentFaction),
		}); err != nil {
			return fmt.Errorf("insert lesson: %w", err)
		}
	}
	return tx.Commit()
}

// QueryMemory fetches exemplar doctrines (to emulate), cautionary doctrines
// (to avoid), and lessons for the librarian to judge. SQL filters only on
// our_faction — the librarian decides cross-opponent relevance semantically,
// so opponent_faction on MemoryFilter is accepted for future use but not
// applied at the SQL layer.
func (s *Store) QueryMemory(ctx context.Context, f MemoryFilter) (MemoryContext, error) {
	if f.TopKExemplars <= 0 {
		f.TopKExemplars = 2
	}
	if f.TopKCautionaries <= 0 {
		f.TopKCautionaries = 2
	}
	if f.TopMLessons <= 0 {
		f.TopMLessons = 5
	}

	// Same-side fan-out (vimy-wkv). sqlc + sqlite mishandles slice params
	// when combined with named params like @lim (positional indices shift),
	// so we run the single-faction query for each side member and merge.
	// Limits are applied per-faction; the librarian's TopK selection layer
	// trims the union to the desired final size.
	sideFactions := SameSideFactions(f.OurFaction)

	var exemplars []db.QueryExemplarDoctrinesRow
	for _, faction := range sideFactions {
		rows, err := s.queries.QueryExemplarDoctrines(ctx, db.QueryExemplarDoctrinesParams{
			OurFaction: faction,
			Lim:        int64(f.TopKExemplars),
		})
		if err != nil {
			return MemoryContext{}, fmt.Errorf("query exemplars: %w", err)
		}
		exemplars = append(exemplars, rows...)
	}

	var cautionaries []db.QueryCautionaryDoctrinesRow
	for _, faction := range sideFactions {
		rows, err := s.queries.QueryCautionaryDoctrines(ctx, db.QueryCautionaryDoctrinesParams{
			OurFaction: faction,
			Lim:        int64(f.TopKCautionaries),
		})
		if err != nil {
			return MemoryContext{}, fmt.Errorf("query cautionaries: %w", err)
		}
		cautionaries = append(cautionaries, rows...)
	}

	var lessons []db.QueryLessonsRow
	for _, faction := range sideFactions {
		rows, err := s.queries.QueryLessons(ctx, db.QueryLessonsParams{
			OurFaction: faction,
			Lim:        int64(f.TopMLessons),
		})
		if err != nil {
			return MemoryContext{}, fmt.Errorf("query lessons: %w", err)
		}
		lessons = append(lessons, rows...)
	}

	out := MemoryContext{
		Exemplars:    make([]ArchivedDoctrine, 0, len(exemplars)),
		Cautionaries: make([]ArchivedDoctrine, 0, len(cautionaries)),
		Lessons:      make([]LessonRow, 0, len(lessons)),
	}
	for _, d := range exemplars {
		out.Exemplars = append(out.Exemplars, archivedDoctrineFromRow(
			d.DoctrineJson, d.Rating, d.RatingReason,
			d.Won, d.QualityTag, d.OpponentFaction, d.PlayedAt,
		))
	}
	for _, d := range cautionaries {
		out.Cautionaries = append(out.Cautionaries, archivedDoctrineFromRow(
			d.DoctrineJson, d.Rating, d.RatingReason,
			d.Won, d.QualityTag, d.OpponentFaction, d.PlayedAt,
		))
	}
	for _, l := range lessons {
		out.Lessons = append(out.Lessons, LessonRow{
			Trigger:    l.TriggerText,
			Guidance:   l.GuidanceText,
			Confidence: l.Confidence,
		})
	}
	return out, nil
}

func archivedDoctrineFromRow(
	doctrineJSON string,
	rating, ratingReason sql.NullString,
	won int64,
	qualityTag, opponentFaction sql.NullString,
	playedAt int64,
) ArchivedDoctrine {
	return ArchivedDoctrine{
		DoctrineJSON:    doctrineJSON,
		Rating:          nullStringOr(rating, ""),
		RatingReason:    nullStringOr(ratingReason, ""),
		Won:             won == 1,
		QualityTag:      nullStringOr(qualityTag, ""),
		OpponentFaction: nullStringOr(opponentFaction, ""),
		PlayedAt:        time.Unix(playedAt, 0),
	}
}

// GameSummary is a dashboard-ready view of an archived game.
type GameSummary struct {
	ID              int64
	PlayedAt        time.Time
	OurFaction      string
	OpponentFaction string
	DurationTicks   int
	Won             bool
	QualityTag      string
	ReviewJSON      string
}

// GlobalLesson is a lesson with its faction context preserved for display.
type GlobalLesson struct {
	Trigger        string
	Guidance       string
	Confidence     float64
	AppliesFaction string
	AppliesVs      string
}

// RecentGames returns up to limit archived games, most recent first.
func (s *Store) RecentGames(ctx context.Context, limit int) ([]GameSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.queries.ListGamesDetailed(ctx, db.ListGamesDetailedParams{
		Limit:  int64(limit),
		Offset: 0,
	})
	if err != nil {
		return nil, err
	}
	out := make([]GameSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, GameSummary{
			ID:              r.ID,
			PlayedAt:        time.Unix(r.PlayedAt, 0),
			OurFaction:      r.OurFaction,
			OpponentFaction: nullStringOr(r.OpponentFaction, ""),
			DurationTicks:   int(r.DurationTicks),
			Won:             r.Won == 1,
			QualityTag:      nullStringOr(r.QualityTag, ""),
			ReviewJSON:      nullStringOr(r.ReviewJson, ""),
		})
	}
	return out, nil
}

// TopLessons returns the highest-confidence lessons across all factions.
func (s *Store) TopLessons(ctx context.Context, limit int) ([]GlobalLesson, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.queries.ListTopLessons(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]GlobalLesson, 0, len(rows))
	for _, r := range rows {
		out = append(out, GlobalLesson{
			Trigger:        r.TriggerText,
			Guidance:       r.GuidanceText,
			Confidence:     r.Confidence,
			AppliesFaction: r.AppliesFaction,
			AppliesVs:      nullStringOr(r.AppliesVs, ""),
		})
	}
	return out, nil
}

// EncodeJSON is a convenience helper so callers outside this package can
// produce the JSON payloads expected by ArchiveGame/ArchiveLessons without
// pulling in encoding/json themselves.
func EncodeJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("store: not found")

// helpers

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullStringOr(ns sql.NullString, def string) string {
	if ns.Valid {
		return ns.String
	}
	return def
}
