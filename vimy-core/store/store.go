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
	// Where the state export for this game was written, when one was. What
	// lets a replay find the states that go with these doctrines.
	ExportPath string
	// The directive the weights were chosen from — a post mortem that stops at
	// the weights stops one step short.
	Directive    string
	InfantryLost int
	VehiclesLost int
	// Only the forward half; home is the subtraction from the totals above.
	InfantryLostForward int
	VehiclesLostForward int

	// The engine's own account, to hold the inferred figures against.
	EngineUnitsKilled     int
	EngineUnitsDead       int
	EngineBuildingsKilled int
	EngineBuildingsDead   int
	EngineKillsCost       int
	EngineDeathsCost      int
	EngineArmyValue       int
	EngineEarned          int

	// What the two armies were worth while the game was live, not at the end.
	HarvesterIdle       int
	HarvesterMining     int
	HarvesterTravelling int
	HarvesterAtRefinery int
	HarvesterHaulDist   float64

	StrikeBlockedUnclumped   int
	StrikeBlockedBlindAtBase int
	StrikeBlockedEnRoute     int
	StrikeBlockedNotBuilding int
	StrikeBlockedOutOfReach  int

	OurArmyPeak       int
	OurArmyMean       int
	EnemyArmySeenPeak int
	EnemyArmySeenMean int
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

// InputDoctrine is what the store needs to archive a doctrine — serialized
// rather than the agent types, so this package imports neither agent nor rules.
type InputDoctrine struct {
	Tick         int
	DoctrineJSON string // already-serialized rules.Doctrine
	Rating       string // "" if unrated
	RatingReason string

	// Rule-engine trace for this doctrine's window.
	RuleSetJSON string            // JSON array of rule names compiled during the window; "" skips storage
	RuleFirings []InputRuleFiring // per-rule firing stats; empty means no rule fired (or pre-instrumentation)
}

// InputRuleFiring is a per-rule, per-doctrine-window firing record.
type InputRuleFiring struct {
	RuleName string
	// FireCount is condition matches, ActCount actions that did something. A rule
	// where the two diverge is the interesting kind.
	FireCount int
	ActCount  int
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

// SameSideFactions returns the factions sharing a tech tree, which is the right
// granularity for memory retrieval. Cross-side stays blocked: the unit rosters
// don't correspond.
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

// MemoryContext is the assembled retrieval result: Exemplars to emulate,
// Cautionaries framed as constraints to avoid rather than as templates.
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

// RecordGame inserts a minimal row when no retrospective is available, so the
// dashboard counters stay correct.
func (s *Store) RecordGame(r GameRecord) error {
	_, err := s.queries.InsertGame(context.Background(), db.InsertGameParams{
		ExportPath:      sql.NullString{},
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

// ArchiveGame inserts the game row and its per-doctrine records, returning the
// new game's ID. reviewJSON and qualityTag are empty when no review ran.
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
		ExportPath:      nullableString(gameCtx.ExportPath),
		Directive:       nullableString(gameCtx.Directive),
		PlayedAt:        time.Now().Unix(),
		OurFaction:      gameCtx.OurFaction,
		OpponentFaction: nullableString(gameCtx.OpponentFaction),
		MapWidth:        int64(gameCtx.MapWidth),
		MapHeight:       int64(gameCtx.MapHeight),
		DurationTicks:   int64(gameCtx.DurationTicks),
		Won:             boolToInt(gameCtx.Won),
		QualityTag:      nullableString(qualityTag),
		ReviewJson:      nullableString(reviewJSON),

		// The inferred columns are left NULL from here. They hold real readings
		// for games 111-120 and the record of how far off they were; writing a
		// zero now would read as a measurement rather than an absence.
		EnemyUnitsKilled:     sql.NullInt64{},
		EnemyBuildingsKilled: sql.NullInt64{},
		EnemyUnitsPresumed:   sql.NullInt64{},
		InfantryLost:         sql.NullInt64{Int64: int64(gameCtx.InfantryLost), Valid: true},
		VehiclesLost:         sql.NullInt64{Int64: int64(gameCtx.VehiclesLost), Valid: true},

		InfantryLostForward: sql.NullInt64{Int64: int64(gameCtx.InfantryLostForward), Valid: true},
		VehiclesLostForward: sql.NullInt64{Int64: int64(gameCtx.VehiclesLostForward), Valid: true},

		EngineUnitsKilled:     sql.NullInt64{Int64: int64(gameCtx.EngineUnitsKilled), Valid: true},
		EngineUnitsDead:       sql.NullInt64{Int64: int64(gameCtx.EngineUnitsDead), Valid: true},
		EngineBuildingsKilled: sql.NullInt64{Int64: int64(gameCtx.EngineBuildingsKilled), Valid: true},
		EngineBuildingsDead:   sql.NullInt64{Int64: int64(gameCtx.EngineBuildingsDead), Valid: true},
		EngineKillsCost:       sql.NullInt64{Int64: int64(gameCtx.EngineKillsCost), Valid: true},
		EngineDeathsCost:      sql.NullInt64{Int64: int64(gameCtx.EngineDeathsCost), Valid: true},
		EngineArmyValue:       sql.NullInt64{Int64: int64(gameCtx.EngineArmyValue), Valid: true},
		EngineEarned:          sql.NullInt64{Int64: int64(gameCtx.EngineEarned), Valid: true},

		HarvesterIdle:         sql.NullInt64{Int64: int64(gameCtx.HarvesterIdle), Valid: true},
		HarvesterMining:       sql.NullInt64{Int64: int64(gameCtx.HarvesterMining), Valid: true},
		HarvesterTravelling:   sql.NullInt64{Int64: int64(gameCtx.HarvesterTravelling), Valid: true},
		HarvesterAtRefinery:   sql.NullInt64{Int64: int64(gameCtx.HarvesterAtRefinery), Valid: true},
		HarvesterHaulDistance: sql.NullFloat64{Float64: gameCtx.HarvesterHaulDist, Valid: true},

		StrikeBlockedUnclumped:   sql.NullInt64{Int64: int64(gameCtx.StrikeBlockedUnclumped), Valid: true},
		StrikeBlockedBlindAtBase: sql.NullInt64{Int64: int64(gameCtx.StrikeBlockedBlindAtBase), Valid: true},
		StrikeBlockedEnRoute:     sql.NullInt64{Int64: int64(gameCtx.StrikeBlockedEnRoute), Valid: true},
		StrikeBlockedNotBuilding: sql.NullInt64{Int64: int64(gameCtx.StrikeBlockedNotBuilding), Valid: true},
		StrikeBlockedOutOfReach:  sql.NullInt64{Int64: int64(gameCtx.StrikeBlockedOutOfReach), Valid: true},

		OurArmyPeak:       sql.NullInt64{Int64: int64(gameCtx.OurArmyPeak), Valid: true},
		OurArmyMean:       sql.NullInt64{Int64: int64(gameCtx.OurArmyMean), Valid: true},
		EnemyArmySeenPeak: sql.NullInt64{Int64: int64(gameCtx.EnemyArmySeenPeak), Valid: true},
		EnemyArmySeenMean: sql.NullInt64{Int64: int64(gameCtx.EnemyArmySeenMean), Valid: true},
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
				// Always Valid — NULL is reserved for rows predating the counter.
				ActCount:  sql.NullInt64{Int64: int64(f.ActCount), Valid: true},
				FirstTick: int64(f.FirstTick),
				LastTick:  int64(f.LastTick),
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

// QueryMemory gathers exemplars, cautionaries and lessons for the librarian to
// judge. SQL filters on our_faction only: cross-opponent relevance is a
// semantic call, so MemoryFilter's opponent_faction is accepted but unused.
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

	// One query per side member rather than a slice param: sqlc and sqlite shift
	// positional indices when a slice is combined with named params. Limits apply
	// per faction; the librarian trims the union.
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

// EncodeJSON produces the payloads ArchiveGame and ArchiveLessons expect.
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

// ReplayableGame is an archived game with a state export beside it — one Currie
// can replay against the rule sets that ran.
type ReplayableGame struct {
	ID              int64
	PlayedAt        time.Time
	OurFaction      string
	OpponentFaction string
	DurationTicks   int
	Won             bool
	QualityTag      string
	ExportPath      string
	Directive       string
}

// ReplayableGames lists games that recorded an export, newest first. Games
// without one are omitted rather than offered and then failed.
func (s *Store) ReplayableGames(ctx context.Context) ([]ReplayableGame, error) {
	rows, err := s.queries.ListReplayableGames(ctx)
	if err != nil {
		return nil, fmt.Errorf("list replayable games: %w", err)
	}
	out := make([]ReplayableGame, 0, len(rows))
	for _, r := range rows {
		out = append(out, ReplayableGame{
			ID:              r.ID,
			PlayedAt:        time.Unix(r.PlayedAt, 0),
			OurFaction:      r.OurFaction,
			OpponentFaction: r.OpponentFaction.String,
			DurationTicks:   int(r.DurationTicks),
			Won:             r.Won != 0,
			QualityTag:      r.QualityTag.String,
			ExportPath:      r.ExportPath.String,
			Directive:       r.Directive.String,
		})
	}
	return out, nil
}

// DoctrineWindow is one doctrine and the tick it took effect.
type DoctrineWindow struct {
	ID           int64
	Tick         int
	DoctrineJSON string
	Rating       string
	RatingReason string
}

// DoctrinesForGame returns a game's doctrine windows in the order they applied.
func (s *Store) DoctrinesForGame(ctx context.Context, gameID int64) ([]DoctrineWindow, error) {
	rows, err := s.queries.ListDoctrinesForGame(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("list doctrines for game %d: %w", gameID, err)
	}
	out := make([]DoctrineWindow, 0, len(rows))
	for _, r := range rows {
		out = append(out, DoctrineWindow{
			ID:           r.ID,
			Tick:         int(r.Tick),
			DoctrineJSON: r.DoctrineJson,
			Rating:       r.Rating.String,
			RatingReason: r.RatingReason.String,
		})
	}
	return out, nil
}

// LinkExport backfills games recorded before the archive tracked export paths;
// new games carry theirs from the moment they are archived.
func (s *Store) LinkExport(ctx context.Context, gameID int64, path string) error {
	if err := s.queries.SetGameExportPath(ctx, db.SetGameExportPathParams{
		ExportPath: nullableString(path),
		ID:         gameID,
	}); err != nil {
		return fmt.Errorf("link export to game %d: %w", gameID, err)
	}
	return nil
}

// AllGames lists every archived game, replayable or not, newest first.
func (s *Store) AllGames(ctx context.Context) ([]ReplayableGame, error) {
	rows, err := s.queries.ListAllGames(ctx)
	if err != nil {
		return nil, fmt.Errorf("list games: %w", err)
	}
	out := make([]ReplayableGame, 0, len(rows))
	for _, r := range rows {
		out = append(out, ReplayableGame{
			ID:              r.ID,
			PlayedAt:        time.Unix(r.PlayedAt, 0),
			OurFaction:      r.OurFaction,
			OpponentFaction: r.OpponentFaction.String,
			DurationTicks:   int(r.DurationTicks),
			Won:             r.Won != 0,
			QualityTag:      r.QualityTag.String,
			ExportPath:      r.ExportPath.String,
			Directive:       r.Directive.String,
		})
	}
	return out, nil
}

// Firing is what a rule did during a game, which a replay cannot recover: a
// replay sees only sampled states, so a rule that fired a handful of times can
// be absent from every sample and read as one that never could.
type Firing struct {
	Matched int
	// Acted is -1 when the game predates the counter, which is not the same as
	// zero and must not be read as "it did nothing".
	Acted int
	// When the rule first and last did something. "It fired" and "it first fired
	// two thirds of the way in" are different findings, and only the second
	// distinguishes a working scouting rule from a broken one.
	FirstTick, LastTick int
}

// FiringsForGame is what every rule did, by name.
func (s *Store) FiringsForGame(ctx context.Context, gameID int64) (map[string]Firing, error) {
	rows, err := s.queries.ListFiringsForGame(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("firings for game %d: %w", gameID, err)
	}
	out := make(map[string]Firing, len(rows))
	for _, r := range rows {
		f := Firing{Acted: -1}
		if r.Matched.Valid {
			f.Matched = int(r.Matched.Float64)
		}
		if r.Acted.Valid {
			f.Acted = int(r.Acted.Float64)
		}
		f.FirstTick, f.LastTick = int(r.FirstTick), int(r.LastTick)
		out[r.RuleName] = f
	}
	return out, nil
}

// Outcome is what the engine recorded about how a game went.
//
// The sidecar never asks the engine anything mid-game — it observes. These are
// the end-of-game statistics the mod hands over, and they are the only numbers
// in the archive that are not Vimy's own opinion of events. Where Vimy's own
// counting disagreed with these, Vimy was wrong: a kill tracker built on
// observation over-counted by 2.9x and was deleted.
type Outcome struct {
	ID              int64
	DurationTicks   int
	Won             bool
	OurFaction      string
	OpponentFaction string

	// What the trade was worth, in credits, both ways.
	KillsCost, DeathsCost int
	BuildingsKilled       int
	Earned                int

	// Peak army value, ours and theirs. Theirs is what was SEEN, so it is a
	// floor and never a measurement — fog hides the rest.
	OurArmyPeak, EnemyArmySeenPeak int

	// Losses, and how many happened away from base. The split is the difference
	// between an army that died attacking and one ground down at home.
	InfantryLost, VehiclesLost               int
	InfantryLostForward, VehiclesLostForward int

	// Where harvester time went, in harvester-samples: one harvester in one
	// state. Shares of these answer what three separate economy changes were
	// guessing at — whether the harvesters are idle, working, walking, or
	// queued at a refinery.
	HarvesterIdle, HarvesterMining           int
	HarvesterTravelling, HarvesterAtRefinery int
	// Mean cells from the nearest refinery while travelling. Rising means the
	// ore near the base is gone.
	HarvesterHaulDistance float64

	// Why squads told to attack never shot a building. Each has a different
	// fix, and the assault phase log cannot tell them apart. blind-at-base is
	// the one that matters: the squad is standing where the enemy is supposed
	// to be and can see nothing.
	StrikeBlockedUnclumped, StrikeBlockedBlindAtBase  int
	StrikeBlockedEnRoute                              int
	StrikeBlockedNotBuilding, StrikeBlockedOutOfReach int

	// Which fields the row actually carried. Games predating a migration have
	// NULL, which is not zero: "destroyed no buildings" and "was not counting
	// buildings" are different findings.
	HasTrade, HasArmy, HasLosses, HasHarvesters, HasStrikes bool
}

// HarvesterSamples is the denominator for the four shares.
func (o Outcome) HarvesterSamples() int {
	return o.HarvesterIdle + o.HarvesterMining + o.HarvesterTravelling + o.HarvesterAtRefinery
}

// HarvesterShare is one phase as a percentage of harvester time.
func (o Outcome) HarvesterShare(n int) int {
	total := o.HarvesterSamples()
	if total == 0 {
		return 0
	}
	return n * 100 / total
}

// VehiclesLostAtHome is the complement of the forward count.
func (o Outcome) VehiclesLostAtHome() int { return o.VehiclesLost - o.VehiclesLostForward }

// InfantryLostAtHome is the complement of the forward count.
func (o Outcome) InfantryLostAtHome() int { return o.InfantryLost - o.InfantryLostForward }

// TradeRatio is credits lost per credit destroyed. Below 1 means the exchange
// was won. Zero when the game recorded no trade.
func (o Outcome) TradeRatio() float64 {
	if o.KillsCost == 0 {
		return 0
	}
	return float64(o.DeathsCost) / float64(o.KillsCost)
}

// GameOutcome reads one game's engine statistics.
func (s *Store) GameOutcome(ctx context.Context, id int64) (Outcome, error) {
	r, err := s.queries.GetGameOutcome(ctx, id)
	if err != nil {
		return Outcome{}, fmt.Errorf("game %d outcome: %w", id, err)
	}
	o := Outcome{
		ID:              r.ID,
		DurationTicks:   int(r.DurationTicks),
		Won:             r.Won != 0,
		OurFaction:      r.OurFaction,
		OpponentFaction: r.OpponentFaction.String,

		KillsCost:           int(r.EngineKillsCost.Int64),
		DeathsCost:          int(r.EngineDeathsCost.Int64),
		BuildingsKilled:     int(r.EngineBuildingsKilled.Int64),
		Earned:              int(r.EngineEarned.Int64),
		OurArmyPeak:         int(r.OurArmyPeak.Int64),
		EnemyArmySeenPeak:   int(r.EnemyArmySeenPeak.Int64),
		InfantryLost:        int(r.InfantryLost.Int64),
		VehiclesLost:        int(r.VehiclesLost.Int64),
		InfantryLostForward: int(r.InfantryLostForward.Int64),
		VehiclesLostForward: int(r.VehiclesLostForward.Int64),

		HarvesterIdle:         int(r.HarvesterIdle.Int64),
		HarvesterMining:       int(r.HarvesterMining.Int64),
		HarvesterTravelling:   int(r.HarvesterTravelling.Int64),
		HarvesterAtRefinery:   int(r.HarvesterAtRefinery.Int64),
		HarvesterHaulDistance: r.HarvesterHaulDistance.Float64,

		StrikeBlockedUnclumped:   int(r.StrikeBlockedUnclumped.Int64),
		StrikeBlockedBlindAtBase: int(r.StrikeBlockedBlindAtBase.Int64),
		StrikeBlockedEnRoute:     int(r.StrikeBlockedEnRoute.Int64),
		StrikeBlockedNotBuilding: int(r.StrikeBlockedNotBuilding.Int64),
		StrikeBlockedOutOfReach:  int(r.StrikeBlockedOutOfReach.Int64),

		HasStrikes:    r.StrikeBlockedUnclumped.Valid,
		HasHarvesters: r.HarvesterMining.Valid,
		HasTrade:      r.EngineKillsCost.Valid && r.EngineDeathsCost.Valid,
		HasArmy:       r.OurArmyPeak.Valid && r.EnemyArmySeenPeak.Valid,
		HasLosses:     r.VehiclesLost.Valid && r.VehiclesLostForward.Valid,
	}
	return o, nil
}
