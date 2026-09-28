package agent

import (
	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
	"testing"
)

func TestDoctrineResponseAfterResetIsDiscarded(t *testing.T) {
	e, _ := rules.NewEngine(nil)
	s := NewStrategist(e, "balanced", 3000)
	s.latest = &model.GameState{Tick: 100}
	generation := s.generation
	d := rules.Doctrine{Name: "old game"}
	prepared, err := rules.PrepareDoctrine(d, []*rules.Rule{{Name: "new", ConditionSrc: "false"}})
	if err != nil {
		t.Fatal(err)
	}
	s.Reset()
	s.latest = &model.GameState{Tick: 20}
	if s.activateDoctrine(generation, d, prepared, nil, false) {
		t.Fatal("accepted previous game's response")
	}
	if len(s.history) != 0 || len(e.RuleNames()) != 0 {
		t.Fatal("stale response changed state")
	}
	if !s.activateDoctrine(s.generation, d, prepared, nil, false) {
		t.Fatal("rejected current response")
	}
	if s.history[0].Tick != 20 || s.lastTick != 20 {
		t.Fatal("history does not record activation tick")
	}
}
