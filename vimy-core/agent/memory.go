package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	baml_client "github.com/nstehr/vimy/vimy-core/baml_client"
	"github.com/nstehr/vimy/vimy-core/baml_client/types"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
)

const (
	memoryTopKExemplars    = 2
	memoryTopKCautionaries = 2
	memoryTopMLessons      = 5
)

// LibrarianSnapshot is a dashboard-friendly capture of the librarian's
// last decision for the current game — alignment note + the reasoning
// behind each kept item. Cleared on Reset.
type LibrarianSnapshot struct {
	AlignmentNote   string
	Directive       string
	OurFaction      string
	OpponentFaction string
	GeneratedAt     time.Time
	Exemplars       []LibrarianExemplar
	Cautionaries    []LibrarianCautionary
	Lessons         []LibrarianLesson
	// Candidate counts show what was available BEFORE filtering.
	CandidateExemplars    int
	CandidateCautionaries int
	CandidateLessons      int
}

type LibrarianExemplar struct {
	Context         string
	DoctrineSummary string
	WhyRelevant     string
	WhatToEmulate   string
}

type LibrarianCautionary struct {
	Context       string
	FailedPattern string
	WhyRelevant   string
	WhatToAvoid   string
}

type LibrarianLesson struct {
	Trigger  string
	Guidance string
}

// GetLibrarianSnapshot returns the last librarian decision, or nil if the
// librarian hasn't run yet this game (or was skipped / failed).
func (s *Strategist) GetLibrarianSnapshot() *LibrarianSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.librarian == nil {
		return nil
	}
	copy := *s.librarian
	return &copy
}

// ensureMemory returns a MemoryContext suitable for injection into the BAML
// doctrine prompt. Cached for the lifetime of the game — cleared on Reset.
// Returns nil if no store is wired or if retrieval/filtering fails.
//
// Flow: QueryMemory gathers a broad candidate set, then SelectRelevantMemory
// (the librarian) filters it down to what's actually aligned with the
// current directive. If the librarian judges nothing relevant, we inject
// nothing — strictly better than anchoring the strategist to misaligned
// precedent.
//
// mapWidth/mapHeight are currently unused but reserved for future map-class
// filtering once enough games accumulate to justify it.
func (s *Strategist) ensureMemory(ctx context.Context, mapWidth, mapHeight int) *types.MemoryContext {
	_ = mapWidth
	_ = mapHeight
	s.mu.Lock()
	if s.memoryAttempted {
		cached := s.memoryCache
		s.mu.Unlock()
		return cached
	}
	st := s.store
	ourFaction := s.faction
	opponentFaction := s.opponentFaction
	directive := s.directive
	s.memoryAttempted = true
	s.mu.Unlock()

	if st == nil || ourFaction == "" {
		return nil
	}
	if opponentFaction == "" {
		opponentFaction = "unknown"
	}

	mem, err := st.QueryMemory(ctx, store.MemoryFilter{
		OurFaction:       ourFaction,
		OpponentFaction:  opponentFaction,
		TopKExemplars:    memoryTopKExemplars,
		TopKCautionaries: memoryTopKCautionaries,
		TopMLessons:      memoryTopMLessons,
	})
	if err != nil {
		slog.Warn("memory query failed", "error", err)
		return nil
	}

	if len(mem.Exemplars) == 0 && len(mem.Cautionaries) == 0 && len(mem.Lessons) == 0 {
		return nil
	}

	bamlMem := s.runLibrarian(ctx, directive, ourFaction, opponentFaction, mem)
	if bamlMem == nil {
		return nil
	}

	s.mu.Lock()
	s.memoryCache = bamlMem
	s.mu.Unlock()

	return bamlMem
}

// runLibrarian invokes SelectRelevantMemory with the candidate set and shapes
// the output into the types.MemoryContext expected by GenerateDoctrine.
// Returns nil (skip injection) on LLM failure or if the librarian selects
// nothing.
func (s *Strategist) runLibrarian(
	ctx context.Context,
	directive, ourFaction, opponentFaction string,
	mem store.MemoryContext,
) *types.MemoryContext {
	candidates := buildCandidates(mem)
	rel, err := baml_client.SelectRelevantMemory(ctx, directive, ourFaction, opponentFaction, candidates)
	if err != nil {
		slog.Warn("librarian call failed; skipping memory injection", "error", err)
		return nil
	}

	out := &types.MemoryContext{
		Exemplar_doctrines:  make([]types.PastDoctrineExample, 0, len(rel.Exemplars)),
		Cautionary_patterns: make([]types.CautionaryPattern, 0, len(rel.Cautionaries)),
		Lessons:             make([]types.MemoryLesson, 0, len(rel.Lesson_ids)),
	}
	snapshot := &LibrarianSnapshot{
		AlignmentNote:         rel.Alignment_note,
		Directive:             directive,
		OurFaction:            ourFaction,
		OpponentFaction:       opponentFaction,
		GeneratedAt:           time.Now(),
		CandidateExemplars:    len(candidates.Exemplars),
		CandidateCautionaries: len(candidates.Cautionaries),
		CandidateLessons:      len(candidates.Lessons),
	}

	exemplarByID := map[int64]types.CandidateExemplar{}
	for _, c := range candidates.Exemplars {
		exemplarByID[c.Id] = c
	}
	for _, sel := range rel.Exemplars {
		c, ok := exemplarByID[sel.Id]
		if !ok {
			continue
		}
		out.Exemplar_doctrines = append(out.Exemplar_doctrines, types.PastDoctrineExample{
			Context:          c.Context,
			Rationale:        sel.What_to_emulate,
			Doctrine_summary: c.Doctrine_summary,
		})
		snapshot.Exemplars = append(snapshot.Exemplars, LibrarianExemplar{
			Context:         c.Context,
			DoctrineSummary: c.Doctrine_summary,
			WhyRelevant:     sel.Why_relevant,
			WhatToEmulate:   sel.What_to_emulate,
		})
	}

	cautionaryByID := map[int64]types.CandidateCautionary{}
	for _, c := range candidates.Cautionaries {
		cautionaryByID[c.Id] = c
	}
	for _, sel := range rel.Cautionaries {
		c, ok := cautionaryByID[sel.Id]
		if !ok {
			continue
		}
		out.Cautionary_patterns = append(out.Cautionary_patterns, types.CautionaryPattern{
			Context:        c.Context,
			Failed_pattern: c.Failed_pattern,
			Failure_reason: sel.What_to_avoid,
		})
		snapshot.Cautionaries = append(snapshot.Cautionaries, LibrarianCautionary{
			Context:       c.Context,
			FailedPattern: c.Failed_pattern,
			WhyRelevant:   sel.Why_relevant,
			WhatToAvoid:   sel.What_to_avoid,
		})
	}

	lessonByID := map[int64]types.CandidateLesson{}
	for _, c := range candidates.Lessons {
		lessonByID[c.Id] = c
	}
	for _, id := range rel.Lesson_ids {
		c, ok := lessonByID[id]
		if !ok {
			continue
		}
		out.Lessons = append(out.Lessons, types.MemoryLesson{
			Trigger:  c.Trigger,
			Guidance: c.Guidance,
		})
		snapshot.Lessons = append(snapshot.Lessons, LibrarianLesson{
			Trigger:  c.Trigger,
			Guidance: c.Guidance,
		})
	}

	s.mu.Lock()
	s.librarian = snapshot
	s.mu.Unlock()

	if len(out.Exemplar_doctrines) == 0 && len(out.Cautionary_patterns) == 0 && len(out.Lessons) == 0 {
		slog.Info("librarian filtered all memory out",
			"alignment_note", rel.Alignment_note,
			"our_faction", ourFaction,
			"opponent", opponentFaction,
			"directive", directive)
		return nil
	}

	slog.Info("memory injected",
		"exemplars", len(out.Exemplar_doctrines),
		"cautionary_patterns", len(out.Cautionary_patterns),
		"lessons", len(out.Lessons),
		"alignment_note", rel.Alignment_note,
		"our_faction", ourFaction,
		"opponent", opponentFaction)
	return out
}

// buildCandidates converts the store retrieval result into the librarian's
// input shape, assigning stable per-call IDs so the librarian's output can
// reference entries back.
func buildCandidates(mem store.MemoryContext) types.MemoryCandidates {
	out := types.MemoryCandidates{
		Exemplars:    make([]types.CandidateExemplar, 0, len(mem.Exemplars)),
		Cautionaries: make([]types.CandidateCautionary, 0, len(mem.Cautionaries)),
		Lessons:      make([]types.CandidateLesson, 0, len(mem.Lessons)),
	}
	nextID := int64(1)
	for _, d := range mem.Exemplars {
		out.Exemplars = append(out.Exemplars, types.CandidateExemplar{
			Id:               nextID,
			Context:          formatDoctrineContext(d),
			Rationale:        extractRationale(d.DoctrineJSON),
			Doctrine_summary: summarizeDoctrineFields(d.DoctrineJSON),
		})
		nextID++
	}
	for _, d := range mem.Cautionaries {
		out.Cautionaries = append(out.Cautionaries, types.CandidateCautionary{
			Id:             nextID,
			Context:        formatCautionaryContext(d),
			Failed_pattern: summarizeDoctrineFields(d.DoctrineJSON),
			Failure_reason: cautionaryFailureReason(d),
		})
		nextID++
	}
	for _, l := range mem.Lessons {
		out.Lessons = append(out.Lessons, types.CandidateLesson{
			Id:         nextID,
			Trigger:    l.Trigger,
			Guidance:   l.Guidance,
			Confidence: l.Confidence,
		})
		nextID++
	}
	return out
}

// toBAMLMemory shapes a store.MemoryContext into the BAML types shape that
// GenerateDoctrine expects. Exemplars carry full context + rationale (to be
// emulated); cautionaries drop the original rationale and substitute the
// reviewer's failure explanation (to be avoided, not templated).
func toBAMLMemory(mem store.MemoryContext) *types.MemoryContext {
	out := &types.MemoryContext{
		Exemplar_doctrines:  make([]types.PastDoctrineExample, 0, len(mem.Exemplars)),
		Cautionary_patterns: make([]types.CautionaryPattern, 0, len(mem.Cautionaries)),
		Lessons:             make([]types.MemoryLesson, 0, len(mem.Lessons)),
	}
	for _, d := range mem.Exemplars {
		out.Exemplar_doctrines = append(out.Exemplar_doctrines, types.PastDoctrineExample{
			Context:          formatDoctrineContext(d),
			Rationale:        extractRationale(d.DoctrineJSON),
			Doctrine_summary: summarizeDoctrineFields(d.DoctrineJSON),
		})
	}
	for _, d := range mem.Cautionaries {
		out.Cautionary_patterns = append(out.Cautionary_patterns, types.CautionaryPattern{
			Context:        formatCautionaryContext(d),
			Failed_pattern: summarizeDoctrineFields(d.DoctrineJSON),
			Failure_reason: cautionaryFailureReason(d),
		})
	}
	for _, l := range mem.Lessons {
		out.Lessons = append(out.Lessons, types.MemoryLesson{
			Trigger:  l.Trigger,
			Guidance: l.Guidance,
		})
	}
	return out
}

func formatCautionaryContext(d store.ArchivedDoctrine) string {
	opp := d.OpponentFaction
	if opp == "" || opp == "unknown" {
		opp = "unknown opponent"
	}
	result := "lost"
	if d.Won {
		result = "won despite weak play"
	}
	return fmt.Sprintf("vs %s, %s", opp, result)
}

func cautionaryFailureReason(d store.ArchivedDoctrine) string {
	if d.RatingReason != "" {
		return d.RatingReason
	}
	return "reviewer flagged this doctrine as weak"
}

func formatDoctrineContext(d store.ArchivedDoctrine) string {
	result := "lost"
	if d.Won {
		result = "won"
	}
	opp := d.OpponentFaction
	if opp == "" {
		opp = "unknown"
	}
	rating := d.Rating
	if rating == "" {
		rating = "unrated"
	}
	return fmt.Sprintf("vs %s, %s — rating: %s", opp, result, rating)
}

func extractRationale(doctrineJSON string) string {
	var d rules.Doctrine
	if err := json.Unmarshal([]byte(doctrineJSON), &d); err != nil {
		return ""
	}
	return d.Rationale
}

func summarizeDoctrineFields(doctrineJSON string) string {
	var d rules.Doctrine
	if err := json.Unmarshal([]byte(doctrineJSON), &d); err != nil {
		return ""
	}
	return fmt.Sprintf(
		"aggression=%.2f econ=%.2f ground_def=%.2f air_def=%.2f tech=%.2f inf=%.2f veh=%.2f air=%.2f",
		d.Aggression, d.EconomyPriority, d.GroundDefensePriority, d.AirDefensePriority,
		d.TechPriority, d.InfantryWeight, d.VehicleWeight, d.AirWeight,
	)
}
