package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nstehr/vimy/vimy-core/baml_client/types"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	s, err := store.NewFromDB(sqlDB)
	if err != nil {
		t.Fatalf("NewFromDB: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleSnapshot() *retrospectiveSnapshot {
	return &retrospectiveSnapshot{
		ourFaction:      "soviet",
		opponentFaction: "allies",
		won:             true,
		durationTicks:   12000,
		mapWidth:        96,
		mapHeight:       96,
		totalLosses:     map[string]int{"infantry": 3, "vehicle": 1},
		history: []DoctrineRecord{
			{Tick: 0, Doctrine: rules.Doctrine{Name: "Opening", Aggression: 0.3}},
			{Tick: 5000, Doctrine: rules.Doctrine{Name: "Pivot", Aggression: 0.8}},
		},
	}
}

func sampleReview() types.GameReview {
	return types.GameReview{
		Summary:     "Won after early eco pivot",
		Quality_tag: "exemplary",
		Doctrine_ratings: []types.DoctrineRating{
			{Name: "Opening", Rating: "adequate", Reasoning: "slow start"},
			{Name: "Pivot", Rating: "strong", Reasoning: "aggressive mid-game closed it out"},
		},
		Lessons: []types.RetroLesson{
			{Trigger_text: "enemy turtles early", Guidance_text: "pressure with vehicles by 5k", Confidence: 0.75},
		},
	}
}

func TestBuildArchivalWithReview(t *testing.T) {
	snap := sampleSnapshot()
	review := sampleReview()

	out := buildArchival(snap, review, true)

	if out.qualityTag != "exemplary" {
		t.Errorf("qualityTag = %q, want 'exemplary'", out.qualityTag)
	}
	if !strings.Contains(out.reviewJSON, "Won after early eco pivot") {
		t.Errorf("reviewJSON missing summary: %s", out.reviewJSON)
	}
	if len(out.doctrines) != 2 {
		t.Fatalf("doctrines len = %d, want 2", len(out.doctrines))
	}
	// Rating attribution by name
	byName := map[string]store.InputDoctrine{}
	for _, d := range out.doctrines {
		var rd rules.Doctrine
		if err := json.Unmarshal([]byte(d.DoctrineJSON), &rd); err != nil {
			t.Fatalf("unmarshal doctrine: %v", err)
		}
		byName[rd.Name] = d
	}
	if byName["Opening"].Rating != "adequate" {
		t.Errorf("Opening rating = %q, want 'adequate'", byName["Opening"].Rating)
	}
	if byName["Pivot"].Rating != "strong" {
		t.Errorf("Pivot rating = %q, want 'strong'", byName["Pivot"].Rating)
	}
	if len(out.lessons) != 1 {
		t.Fatalf("lessons len = %d, want 1", len(out.lessons))
	}
	if out.lessons[0].Confidence != 0.75 {
		t.Errorf("lesson confidence = %v, want 0.75", out.lessons[0].Confidence)
	}
}

func TestBuildArchivalWithoutReview(t *testing.T) {
	snap := sampleSnapshot()
	out := buildArchival(snap, types.GameReview{}, false)

	if out.qualityTag != "" {
		t.Errorf("qualityTag = %q, want empty", out.qualityTag)
	}
	if out.reviewJSON != "" {
		t.Errorf("reviewJSON = %q, want empty", out.reviewJSON)
	}
	if len(out.lessons) != 0 {
		t.Errorf("lessons len = %d, want 0", len(out.lessons))
	}
	if len(out.doctrines) != 2 {
		t.Fatalf("doctrines len = %d, want 2 (unrated)", len(out.doctrines))
	}
	for _, d := range out.doctrines {
		if d.Rating != "" {
			t.Errorf("expected unrated doctrine, got rating=%q", d.Rating)
		}
	}
}

func TestArchivalEndToEnd(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	snap := sampleSnapshot()
	snap.store = s
	review := sampleReview()

	out := buildArchival(snap, review, true)
	gameID, err := s.ArchiveGame(ctx, out.gameCtx, out.qualityTag, out.reviewJSON, out.doctrines)
	if err != nil {
		t.Fatalf("ArchiveGame: %v", err)
	}
	if err := s.ArchiveLessons(ctx, gameID, snap.ourFaction, snap.opponentFaction, out.lessons); err != nil {
		t.Fatalf("ArchiveLessons: %v", err)
	}

	mem, err := s.QueryMemory(ctx, store.MemoryFilter{
		OurFaction:       "soviet",
		OpponentFaction:  "allies",
		TopKExemplars:    5,
		TopKCautionaries: 5,
		TopMLessons:      5,
	})
	if err != nil {
		t.Fatalf("QueryMemory: %v", err)
	}
	// sampleReview() rates "Opening"=adequate + "Pivot"=strong; both go into
	// Exemplars (exemplary game + passing ratings). No weak ratings → no
	// cautionaries.
	if len(mem.Exemplars) != 2 {
		t.Errorf("Exemplars len = %d, want 2", len(mem.Exemplars))
	}
	if len(mem.Cautionaries) != 0 {
		t.Errorf("Cautionaries len = %d, want 0", len(mem.Cautionaries))
	}
	if len(mem.Lessons) != 1 {
		t.Errorf("Lessons len = %d, want 1", len(mem.Lessons))
	}
}
