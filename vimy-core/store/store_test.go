package store

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open :memory:: %v", err)
	}
	s, err := NewFromDB(sqlDB)
	if err != nil {
		t.Fatalf("NewFromDB: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	if err := s.migrate(); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestCountWinsLossesEmpty(t *testing.T) {
	s := newTestStore(t)
	if got := s.Wins(); got != 0 {
		t.Errorf("Wins on empty = %d, want 0", got)
	}
	if got := s.Losses(); got != 0 {
		t.Errorf("Losses on empty = %d, want 0", got)
	}
}

func TestRecordGameLegacyShape(t *testing.T) {
	s := newTestStore(t)
	if err := s.RecordGame(GameRecord{Faction: "soviet", Won: true}); err != nil {
		t.Fatalf("RecordGame: %v", err)
	}
	if err := s.RecordGame(GameRecord{Faction: "allies", Won: false}); err != nil {
		t.Fatalf("RecordGame: %v", err)
	}
	if got := s.Wins(); got != 1 {
		t.Errorf("Wins = %d, want 1", got)
	}
	if got := s.Losses(); got != 1 {
		t.Errorf("Losses = %d, want 1", got)
	}
	games := s.Games()
	if len(games) != 2 {
		t.Fatalf("Games len = %d, want 2", len(games))
	}
}

func TestArchiveAndQueryRoundtrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	gameID, err := s.ArchiveGame(ctx, GameContext{
		OurFaction:      "soviet",
		OpponentFaction: "allies",
		MapWidth:        96,
		MapHeight:       96,
		DurationTicks:   12000,
		Won:             true,
	}, "exemplary", `{"summary":"won quickly"}`, []InputDoctrine{
		{Tick: 0, DoctrineJSON: `{"name":"opening"}`, Rating: "strong", RatingReason: "scouted fast"},
		{Tick: 5000, DoctrineJSON: `{"name":"pivot"}`, Rating: "adequate"},
		{Tick: 10000, DoctrineJSON: `{"name":"closing"}`, Rating: "strong"},
	})
	if err != nil {
		t.Fatalf("ArchiveGame: %v", err)
	}
	if gameID == 0 {
		t.Fatal("expected non-zero gameID")
	}

	if err := s.ArchiveLessons(ctx, gameID, "soviet", "allies", []InputLesson{
		{Trigger: "enemy builds air early", Guidance: "invest in AA by tick 4000", Confidence: 0.8},
		{Trigger: "faced tesla coils", Guidance: "tech to air or mammoths", Confidence: 0.6},
	}); err != nil {
		t.Fatalf("ArchiveLessons: %v", err)
	}

	mem, err := s.QueryMemory(ctx, MemoryFilter{
		OurFaction:       "soviet",
		OpponentFaction:  "allies",
		TopKExemplars:    5,
		TopKCautionaries: 5,
		TopMLessons:      5,
	})
	if err != nil {
		t.Fatalf("QueryMemory: %v", err)
	}
	// The fixture is an 'exemplary' game with three doctrines rated
	// 'strong'/'adequate'/'strong'. All belong in Exemplars, none in
	// Cautionaries.
	if len(mem.Exemplars) != 3 {
		t.Errorf("Exemplars len = %d, want 3", len(mem.Exemplars))
	}
	if len(mem.Cautionaries) != 0 {
		t.Errorf("Cautionaries len = %d, want 0 (no weak-rated doctrines)", len(mem.Cautionaries))
	}
	if len(mem.Lessons) != 2 {
		t.Errorf("Lessons len = %d, want 2", len(mem.Lessons))
	}
	if mem.Lessons[0].Confidence < mem.Lessons[1].Confidence {
		t.Errorf("lessons not ordered by confidence DESC: %+v", mem.Lessons)
	}
}

func TestRetrievalSeparatesExemplarsFromCautionaries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.ArchiveGame(ctx, GameContext{
		OurFaction: "soviet", OpponentFaction: "allies", Won: false,
	}, "cautionary", "", []InputDoctrine{
		{Tick: 100, DoctrineJSON: `{"name":"bad"}`, Rating: "weak", RatingReason: "overcommitted"},
	}); err != nil {
		t.Fatalf("ArchiveGame cautionary: %v", err)
	}

	if _, err := s.ArchiveGame(ctx, GameContext{
		OurFaction: "soviet", OpponentFaction: "allies", Won: true,
	}, "exemplary", "", []InputDoctrine{
		{Tick: 100, DoctrineJSON: `{"name":"good"}`, Rating: "strong"},
	}); err != nil {
		t.Fatalf("ArchiveGame exemplary: %v", err)
	}

	mem, err := s.QueryMemory(ctx, MemoryFilter{
		OurFaction: "soviet", OpponentFaction: "allies",
		TopKExemplars: 10, TopKCautionaries: 10,
	})
	if err != nil {
		t.Fatalf("QueryMemory: %v", err)
	}
	if len(mem.Exemplars) != 1 || mem.Exemplars[0].QualityTag != "exemplary" {
		t.Errorf("Exemplars = %+v, want 1 exemplary", mem.Exemplars)
	}
	if len(mem.Cautionaries) != 1 || mem.Cautionaries[0].Rating != "weak" {
		t.Errorf("Cautionaries = %+v, want 1 weak-rated", mem.Cautionaries)
	}
}

func TestRetrievalUnknownOpponentWidens(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.ArchiveGame(ctx, GameContext{
		OurFaction: "soviet", OpponentFaction: "allies", Won: true,
	}, "exemplary", "", []InputDoctrine{
		{Tick: 100, DoctrineJSON: `{"name":"vs-allies"}`, Rating: "strong"},
	})
	if err != nil {
		t.Fatalf("archive: %v", err)
	}

	mem, err := s.QueryMemory(ctx, MemoryFilter{
		OurFaction:      "soviet",
		OpponentFaction: "unknown",
		TopKExemplars:   5,
	})
	if err != nil {
		t.Fatalf("QueryMemory: %v", err)
	}
	if len(mem.Exemplars) != 1 {
		t.Errorf("unknown-opponent should still retrieve; got %d exemplars", len(mem.Exemplars))
	}
}

func TestRetrievalFiltersOnOurFaction(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.ArchiveGame(ctx, GameContext{
		OurFaction: "allies", OpponentFaction: "soviet", Won: true,
	}, "exemplary", "", []InputDoctrine{
		{Tick: 100, DoctrineJSON: `{"name":"allies-game"}`, Rating: "strong"},
	})
	if err != nil {
		t.Fatalf("archive: %v", err)
	}

	mem, err := s.QueryMemory(ctx, MemoryFilter{
		OurFaction: "soviet", OpponentFaction: "soviet", TopKExemplars: 5,
	})
	if err != nil {
		t.Fatalf("QueryMemory: %v", err)
	}
	if len(mem.Exemplars) != 0 {
		t.Errorf("wrong-faction retrieval leaked: got %d exemplars", len(mem.Exemplars))
	}
}

// Firing counters survive the round trip, and the two counts stay distinct.
//
// act_count is nullable so that rows predating the counter read as "not
// measured" rather than "matched and never acted"; anything written here was
// measured, so it must come back non-NULL — including a genuine zero.
func TestArchiveKeepsMatchedAndActedApart(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	gameID, err := s.ArchiveGame(ctx, GameContext{
		OurFaction: "england", MapWidth: 91, MapHeight: 91, DurationTicks: 100,
	}, "cautionary", "{}", []InputDoctrine{{
		Tick:         0,
		DoctrineJSON: `{"name":"opening"}`,
		RuleFirings: []InputRuleFiring{
			{RuleName: "does-something", FireCount: 9, ActCount: 9, FirstTick: 1, LastTick: 90},
			{RuleName: "does-nothing", FireCount: 12, ActCount: 0, FirstTick: 2, LastTick: 95},
		},
	}})
	if err != nil {
		t.Fatalf("ArchiveGame: %v", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT rule_name, fire_count, act_count FROM rule_firings
		 JOIN archived_doctrines ON archived_doctrines.id = rule_firings.doctrine_id
		 WHERE archived_doctrines.game_id = ? ORDER BY rule_name`, gameID)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	got := map[string][2]any{}
	for rows.Next() {
		var name string
		var fire int
		var act sql.NullInt64
		if err := rows.Scan(&name, &fire, &act); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[name] = [2]any{fire, act}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	idle, ok := got["does-nothing"]
	if !ok {
		t.Fatal("does-nothing was not stored")
	}
	if idle[0] != 12 {
		t.Errorf("does-nothing fire_count = %v, want 12", idle[0])
	}
	act := idle[1].(sql.NullInt64)
	if !act.Valid {
		t.Error("act_count is NULL for a row this code measured")
	}
	if act.Int64 != 0 {
		t.Errorf("does-nothing act_count = %d, want 0", act.Int64)
	}

	worked := got["does-something"][1].(sql.NullInt64)
	if !worked.Valid || worked.Int64 != 9 {
		t.Errorf("does-something act_count = %v, want 9", worked)
	}
}

// The export path reaches the archive, so a replay can find the states that go
// with a game's doctrines without a human pairing files by timestamp.
func TestArchiveRecordsTheExportPath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	const path = "/Users/x/.vimy/exports/export-20260907-201419.json"
	if _, err := s.ArchiveGame(ctx, GameContext{
		OurFaction: "england", MapWidth: 91, MapHeight: 91,
		DurationTicks: 30300, ExportPath: path,
	}, "cautionary", "{}", []InputDoctrine{
		{Tick: 0, DoctrineJSON: `{"name":"opening"}`},
	}); err != nil {
		t.Fatalf("ArchiveGame: %v", err)
	}

	games, err := s.queries.ListReplayableGames(ctx)
	if err != nil {
		t.Fatalf("ListReplayableGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("replayable games = %d, want 1", len(games))
	}
	if got := games[0].ExportPath.String; got != path {
		t.Errorf("export_path = %q, want %q", got, path)
	}
}

// A game recorded without --export-states is not replayable and must not be
// offered as though it were.
func TestGameWithoutAnExportIsNotReplayable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.ArchiveGame(ctx, GameContext{
		OurFaction: "england", MapWidth: 91, MapHeight: 91, DurationTicks: 100,
	}, "cautionary", "{}", []InputDoctrine{
		{Tick: 0, DoctrineJSON: `{"name":"opening"}`},
	}); err != nil {
		t.Fatalf("ArchiveGame: %v", err)
	}

	games, err := s.queries.ListReplayableGames(ctx)
	if err != nil {
		t.Fatalf("ListReplayableGames: %v", err)
	}
	if len(games) != 0 {
		t.Errorf("replayable games = %d, want 0 — it has no export", len(games))
	}
}

// The directive is archived with the game.
//
// The strategist turns a line of prose into weights, and the weights decide
// which rules can fire — so a post mortem that stops at the weights stops one
// step short of the thing a person can actually edit.
func TestArchiveRecordsTheDirective(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	const directive = "air superiority supported with long range rockets"
	if _, err := s.ArchiveGame(ctx, GameContext{
		OurFaction: "england", MapWidth: 91, MapHeight: 91, DurationTicks: 100,
		ExportPath: "/tmp/export.json", Directive: directive,
	}, "cautionary", "{}", []InputDoctrine{{Tick: 0, DoctrineJSON: `{"name":"opening"}`}}); err != nil {
		t.Fatalf("ArchiveGame: %v", err)
	}

	games, err := s.ReplayableGames(ctx)
	if err != nil {
		t.Fatalf("ReplayableGames: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("games = %d, want 1", len(games))
	}
	if games[0].Directive != directive {
		t.Errorf("directive = %q, want %q", games[0].Directive, directive)
	}
}

// A game played before the directive was recorded reads as empty rather than
// as something invented.
func TestGameWithoutADirectiveReadsEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.ArchiveGame(ctx, GameContext{
		OurFaction: "england", MapWidth: 91, MapHeight: 91, DurationTicks: 100,
		ExportPath: "/tmp/export.json",
	}, "cautionary", "{}", []InputDoctrine{{Tick: 0, DoctrineJSON: `{"name":"opening"}`}}); err != nil {
		t.Fatalf("ArchiveGame: %v", err)
	}

	games, _ := s.ReplayableGames(ctx)
	if games[0].Directive != "" {
		t.Errorf("directive = %q, want empty", games[0].Directive)
	}
}
