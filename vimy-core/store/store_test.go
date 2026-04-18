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
