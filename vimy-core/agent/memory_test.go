package agent

import (
	"strings"
	"testing"

	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
)

func TestFormatDoctrineContextWin(t *testing.T) {
	d := store.ArchivedDoctrine{Won: true, OpponentFaction: "allies", Rating: "strong"}
	got := formatDoctrineContext(d)
	if !strings.Contains(got, "vs allies") || !strings.Contains(got, "won") || !strings.Contains(got, "strong") {
		t.Errorf("context missing expected pieces: %q", got)
	}
}

func TestFormatDoctrineContextMissingFields(t *testing.T) {
	d := store.ArchivedDoctrine{Won: false}
	got := formatDoctrineContext(d)
	if !strings.Contains(got, "unknown") || !strings.Contains(got, "lost") || !strings.Contains(got, "unrated") {
		t.Errorf("expected unknown/lost/unrated fallbacks: %q", got)
	}
}

func TestSummarizeDoctrineFields(t *testing.T) {
	dj := mustMarshalDoctrine(t, rules.Doctrine{
		Aggression: 0.8, EconomyPriority: 0.3, VehicleWeight: 0.7,
	})
	got := summarizeDoctrineFields(dj)
	if !strings.Contains(got, "aggression=0.80") {
		t.Errorf("missing aggression: %q", got)
	}
	if !strings.Contains(got, "econ=0.30") {
		t.Errorf("missing econ: %q", got)
	}
	if !strings.Contains(got, "veh=0.70") {
		t.Errorf("missing vehicle: %q", got)
	}
}

func TestToBAMLMemoryShape(t *testing.T) {
	mem := store.MemoryContext{
		Exemplars: []store.ArchivedDoctrine{
			{
				Won:             true,
				OpponentFaction: "allies",
				Rating:          "strong",
				QualityTag:      "exemplary",
				DoctrineJSON:    mustMarshalDoctrine(t, rules.Doctrine{Name: "Rush", Rationale: "hit early"}),
			},
		},
		Cautionaries: []store.ArchivedDoctrine{
			{
				Won:             false,
				OpponentFaction: "england",
				Rating:          "weak",
				RatingReason:    "base overrun before economy stabilized",
				QualityTag:      "cautionary",
				DoctrineJSON:    mustMarshalDoctrine(t, rules.Doctrine{Name: "Bad Rush", Rationale: "LOOK AT MY SEDUCTIVE REASONING", Aggression: 0.9, InfantryWeight: 0.8}),
			},
		},
		Lessons: []store.LessonRow{
			{Trigger: "enemy turtles", Guidance: "pressure mid", Confidence: 0.9},
		},
	}
	out := toBAMLMemory(mem)
	if len(out.Exemplar_doctrines) != 1 {
		t.Fatalf("exemplar_doctrines len = %d", len(out.Exemplar_doctrines))
	}
	if out.Exemplar_doctrines[0].Rationale != "hit early" {
		t.Errorf("exemplar rationale = %q", out.Exemplar_doctrines[0].Rationale)
	}
	if len(out.Cautionary_patterns) != 1 {
		t.Fatalf("cautionary_patterns len = %d", len(out.Cautionary_patterns))
	}
	caution := out.Cautionary_patterns[0]
	// Cautionary MUST drop the original rationale — it would seduce the reader.
	if strings.Contains(caution.Failed_pattern, "SEDUCTIVE") {
		t.Errorf("cautionary pattern leaked seductive rationale: %q", caution.Failed_pattern)
	}
	if caution.Failure_reason != "base overrun before economy stabilized" {
		t.Errorf("cautionary failure_reason = %q, want reviewer's reason", caution.Failure_reason)
	}
	if !strings.Contains(caution.Context, "england") || !strings.Contains(caution.Context, "lost") {
		t.Errorf("cautionary context missing england/lost: %q", caution.Context)
	}
	if len(out.Lessons) != 1 {
		t.Fatalf("lessons len = %d", len(out.Lessons))
	}
	if out.Lessons[0].Trigger != "enemy turtles" {
		t.Errorf("lesson trigger = %q", out.Lessons[0].Trigger)
	}
}

func mustMarshalDoctrine(t *testing.T, d rules.Doctrine) string {
	t.Helper()
	s, err := store.EncodeJSON(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return s
}
