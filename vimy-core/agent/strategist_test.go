package agent

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
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

func TestComputePressureFlags_SlowRushTriggers(t *testing.T) {
	// A first harvester attack near tick 10000 against a still-small base is a
	// slow-tempo rush, not mid-game harassment.
	gs := &model.GameState{Buildings: make([]model.Building, 5)}
	stress := []Event{{Kind: EventHarvesterUnderAttack, Tick: 9980}}
	rushed, _ := computePressureFlags(10000, gs, stress)
	if !rushed {
		t.Errorf("expected being_rushed=true at tick 10000 with 5 buildings + recent harvester attack (slow-rush case)")
	}
}

func TestComputePressureFlags_RushClearsAfterTickWindow(t *testing.T) {
	gs := &model.GameState{Buildings: make([]model.Building, 8)}
	stress := []Event{{Kind: EventHarvesterUnderAttack, Tick: 11000}}
	rushed, _ := computePressureFlags(11500, gs, stress)
	if rushed {
		t.Errorf("expected being_rushed=false past the tick cutoff (11500 > 10500)")
	}
}

func TestComputePressureFlags_HarvesterHarassed(t *testing.T) {
	gs := &model.GameState{Buildings: make([]model.Building, 15)}
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
	gs := &model.GameState{Buildings: make([]model.Building, 15)}
	stress := []Event{
		{Kind: EventHarvesterUnderAttack, Tick: 8000},
		{Kind: EventHarvesterUnderAttack, Tick: 8500},
	}
	_, harassed := computePressureFlags(15000, gs, stress)
	if harassed {
		t.Errorf("expected harvester_harassed=false when events are >2000 ticks old")
	}
}

func TestComputeBurnedAxes_AirBurnsAfterOneCounter(t *testing.T) {
	// Single air-dominant pivot followed by an aircraft-vs-SAM counter is
	// enough to burn the air axis. SAM clusters don't move; one confirmed
	// counter is sufficient.
	history := []DoctrineRecord{
		{Tick: 1000, Doctrine: doctrineWithAir(0.6)},
	}
	stress := []Event{
		{Kind: EventStrategyCountered, Tick: 1200, Detail: "aircraft taking heavy losses vs SAM Site"},
	}
	burned := computeBurnedAxes(history, stress, 2000)
	if !containsAxis(burned, "air") {
		t.Errorf("expected air burned after single SAM counter, got %v", burned)
	}
}

func TestComputeBurnedAxes_VehicleNeedsTwoCounters(t *testing.T) {
	// One vehicle-dominant pivot + one army_devastated event is NOT enough
	// to burn the vehicle axis — could be a single bad engagement, not a
	// hard counter.
	history := []DoctrineRecord{
		{Tick: 1000, Doctrine: doctrineWithVehicle(0.6)},
	}
	stress := []Event{
		{Kind: EventArmyDevastated, Tick: 1200, Detail: "army devastated"},
	}
	burned := computeBurnedAxes(history, stress, 2000)
	if containsAxis(burned, "vehicle") {
		t.Errorf("expected vehicle NOT burned after a single counter, got %v", burned)
	}
}

func TestComputeBurnedAxes_VehicleBurnsAfterTwoCounters(t *testing.T) {
	history := []DoctrineRecord{
		{Tick: 1000, Doctrine: doctrineWithVehicle(0.6)},
		{Tick: 5000, Doctrine: doctrineWithVehicle(0.6)},
	}
	stress := []Event{
		{Kind: EventArmyDevastated, Tick: 1200, Detail: "army devastated"},
		{Kind: EventArmyDevastated, Tick: 5300, Detail: "army devastated"},
	}
	burned := computeBurnedAxes(history, stress, 2000)
	if !containsAxis(burned, "vehicle") {
		t.Errorf("expected vehicle burned after two counters, got %v", burned)
	}
}

func TestComputeBurnedAxes_AirUnburnsAfterRecoveryWindow(t *testing.T) {
	// Air burned at tick 1200 (one SAM kill). At tick 7000, > 5000 ticks
	// later, no further air counter. Air should un-burn so an air-superiority
	// directive can resume aircraft commitment after SEAD.
	history := []DoctrineRecord{
		{Tick: 1000, Doctrine: doctrineWithAir(0.6)},
	}
	stress := []Event{
		{Kind: EventStrategyCountered, Tick: 1200, Detail: "aircraft taking heavy losses vs SAM Site"},
	}
	if got := computeBurnedAxes(history, stress, 2000); !containsAxis(got, "air") {
		t.Errorf("expected air burned shortly after counter (tick 2000), got %v", got)
	}
	if got := computeBurnedAxes(history, stress, 7000); containsAxis(got, "air") {
		t.Errorf("expected air un-burned after 5000-tick recovery (tick 7000), got %v", got)
	}
}

func TestComputeBurnedAxes_VehicleStaysBurnedAcrossMatch(t *testing.T) {
	// Vehicle has no recovery configured — once 2+ counters fire, burned for
	// the rest of the match regardless of how much time passes.
	history := []DoctrineRecord{
		{Tick: 1000, Doctrine: doctrineWithVehicle(0.6)},
		{Tick: 5000, Doctrine: doctrineWithVehicle(0.6)},
	}
	stress := []Event{
		{Kind: EventArmyDevastated, Tick: 1200, Detail: "army devastated"},
		{Kind: EventArmyDevastated, Tick: 5300, Detail: "army devastated"},
	}
	if got := computeBurnedAxes(history, stress, 50000); !containsAxis(got, "vehicle") {
		t.Errorf("expected vehicle still burned at tick 50000 (no recovery), got %v", got)
	}
}

func doctrineWithAir(w float64) rules.Doctrine {
	return rules.Doctrine{AirWeight: w}
}

func doctrineWithVehicle(w float64) rules.Doctrine {
	return rules.Doctrine{VehicleWeight: w}
}

func containsAxis(axes []string, target string) bool {
	for _, a := range axes {
		if a == target {
			return true
		}
	}
	return false
}

func TestComputePressureFlags_BothFalseQuiet(t *testing.T) {
	gs := &model.GameState{Buildings: make([]model.Building, 8)}
	rushed, harassed := computePressureFlags(10000, gs, nil)
	if rushed || harassed {
		t.Errorf("expected both flags false with no stress events, got rushed=%v harassed=%v", rushed, harassed)
	}
}
