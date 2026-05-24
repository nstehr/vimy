package agent

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func TestComputePressureFlags_Rush(t *testing.T) {
	gs := &model.GameState{Buildings: make([]model.Building, 4)}
	stress := []Event{
		{Kind: EventHarvesterUnderAttack, Tick: 3500},
	}
	rushed, harassed := computePressureFlags(4000, gs, stress)
	if !rushed {
		t.Errorf("expected being_rushed=true at tick 4000 with 4 buildings + harvester attack")
	}
	if harassed {
		t.Errorf("expected harvester_harassed=false during rush window")
	}
}

func TestComputePressureFlags_RushClearsWhenBaseGrows(t *testing.T) {
	gs := &model.GameState{Buildings: make([]model.Building, 9)}
	stress := []Event{{Kind: EventHarvesterUnderAttack, Tick: 4000}}
	rushed, _ := computePressureFlags(4500, gs, stress)
	if rushed {
		t.Errorf("expected being_rushed=false once base has 9 buildings")
	}
}

func TestComputePressureFlags_HarvesterHarassed(t *testing.T) {
	gs := &model.GameState{Buildings: make([]model.Building, 12)}
	stress := []Event{
		{Kind: EventHarvesterUnderAttack, Tick: 14000},
		{Kind: EventHarvesterUnderAttack, Tick: 14500},
	}
	rushed, harassed := computePressureFlags(15000, gs, stress)
	if rushed {
		t.Errorf("expected being_rushed=false at tick 15000")
	}
	if !harassed {
		t.Errorf("expected harvester_harassed=true with 2 harvester attacks in window")
	}
}

func TestComputePressureFlags_StaleEventsIgnored(t *testing.T) {
	gs := &model.GameState{Buildings: make([]model.Building, 12)}
	stress := []Event{
		{Kind: EventHarvesterUnderAttack, Tick: 8000},
		{Kind: EventHarvesterUnderAttack, Tick: 8500},
	}
	_, harassed := computePressureFlags(15000, gs, stress)
	if harassed {
		t.Errorf("expected harvester_harassed=false when events are >2000 ticks old")
	}
}

func TestComputePressureFlags_BothFalseQuiet(t *testing.T) {
	gs := &model.GameState{Buildings: make([]model.Building, 8)}
	rushed, harassed := computePressureFlags(10000, gs, nil)
	if rushed || harassed {
		t.Errorf("expected both flags false with no stress events, got rushed=%v harassed=%v", rushed, harassed)
	}
}
