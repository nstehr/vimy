package agent

import (
	"context"
	"fmt"
	"github.com/nstehr/vimy/vimy-core/rules"
)

func (s *Strategist) prepareDoctrine(ctx context.Context, d rules.Doctrine) (*rules.PreparedDoctrine, error) {
	s.mu.Lock()
	c := s.compiler
	s.mu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("no compiler configured")
	}
	compiled, err := c.CompileDetailed(ctx, d)
	if err != nil {
		return nil, err
	}
	return rules.PrepareCompilation(d, compiled)
}

// activateDoctrine joins the engine's activation boundary to the history that
// is archived at game end. A model response from a previous game is discarded.
func (s *Strategist) activateDoctrine(generation uint64, d rules.Doctrine, prepared *rules.PreparedDoctrine, events []Event, hasIntel bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.generation || s.latest == nil {
		return false
	}
	activation := s.engine.Activate(prepared)
	if n := len(s.history); n > 0 {
		s.history[n-1].RuleStats = activation.PreviousStats
	}
	record := DoctrineRecord{Tick: s.latest.Tick, Doctrine: d, Events: events, HasEnemyIntel: hasIntel}
	if activation.Tracing {
		record.RuleSet = activation.RuleNames
	}
	s.history = append(s.history, record)
	s.lastTick = s.latest.Tick
	return true
}
