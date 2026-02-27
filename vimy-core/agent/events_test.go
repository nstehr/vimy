package agent

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
)

// baseGameState returns a minimal game state for testing.
func baseGameState(tick int) model.GameState {
	return model.GameState{
		Tick: tick,
		Player: model.Player{
			Cash:      500,
			Resources: 500,
		},
		Buildings: []model.Building{
			{ID: 1, Type: "fact", HP: 1000, MaxHP: 1000},
			{ID: 2, Type: "powr", HP: 400, MaxHP: 400},
			{ID: 3, Type: "proc", HP: 600, MaxHP: 600},
			{ID: 4, Type: "weap", HP: 800, MaxHP: 800},
		},
		Units: []model.Unit{
			{ID: 10, Type: "harv", Idle: false},
			{ID: 11, Type: "3tnk", Idle: true},
			{ID: 12, Type: "3tnk", Idle: true},
			{ID: 13, Type: "3tnk", Idle: true},
			{ID: 14, Type: "3tnk", Idle: true},
			{ID: 15, Type: "3tnk", Idle: true},
			{ID: 16, Type: "3tnk", Idle: true},
			{ID: 17, Type: "e1", Idle: true},
			{ID: 18, Type: "e1", Idle: true},
		},
	}
}

func TestDetectEvents_NoEvents(t *testing.T) {
	gs := baseGameState(100)
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Same state next tick — no events
	gs.Tick = 101
	events := detectEvents(gs, memory, &prev)
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d: %+v", len(events), events)
	}
}

func TestDetectEvents_NilPrev(t *testing.T) {
	gs := baseGameState(100)
	memory := make(map[string]any)
	events := detectEvents(gs, memory, nil)
	if events != nil {
		t.Errorf("expected nil events for nil prev, got %+v", events)
	}
}

func TestDetectEvents_CriticalBuildingLost(t *testing.T) {
	gs := baseGameState(100)
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Remove construction yard
	gs.Tick = 101
	gs.Buildings = gs.Buildings[1:] // remove fact (id 1)

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventCriticalBuildingLost {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected critical_building_lost event, got %+v", events)
	}
}

func TestDetectEvents_CriticalBuildingLost_FactionVariant(t *testing.T) {
	gs := baseGameState(100)
	// Use a faction variant like "fact.england"
	gs.Buildings[0] = model.Building{ID: 1, Type: "fact.england", HP: 1000, MaxHP: 1000}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Remove it
	gs.Tick = 101
	gs.Buildings = gs.Buildings[1:]

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventCriticalBuildingLost {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected critical_building_lost for faction variant, got %+v", events)
	}
}

func TestDetectEvents_NonCriticalBuildingLost(t *testing.T) {
	gs := baseGameState(100)
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Remove power plant (non-critical)
	gs.Tick = 101
	gs.Buildings = []model.Building{gs.Buildings[0], gs.Buildings[2], gs.Buildings[3]}

	events := detectEvents(gs, memory, &prev)
	for _, e := range events {
		if e.Kind == EventCriticalBuildingLost {
			t.Errorf("did not expect critical_building_lost for power plant, got %+v", events)
		}
	}
}

func TestDetectEvents_ArmyDevastated(t *testing.T) {
	gs := baseGameState(100)
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill >50% of combat units (8 combat units → keep 3)
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv", Idle: false}, // harvester doesn't count
		{ID: 11, Type: "3tnk", Idle: true},
		{ID: 17, Type: "e1", Idle: true},
		{ID: 18, Type: "e1", Idle: true},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventArmyDevastated {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected army_devastated event, got %+v", events)
	}
}

func TestDetectEvents_ArmyDevastated_BelowFloor(t *testing.T) {
	// Only 4 combat units — below floor of 6, should not trigger
	gs := model.GameState{
		Tick:   100,
		Player: model.Player{Cash: 500, Resources: 500},
		Units: []model.Unit{
			{ID: 11, Type: "3tnk"},
			{ID: 12, Type: "3tnk"},
			{ID: 13, Type: "3tnk"},
			{ID: 14, Type: "3tnk"},
		},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill all but one
	gs.Tick = 101
	gs.Units = []model.Unit{{ID: 11, Type: "3tnk"}}

	events := detectEvents(gs, memory, &prev)
	for _, e := range events {
		if e.Kind == EventArmyDevastated {
			t.Errorf("did not expect army_devastated below floor, got %+v", events)
		}
	}
}

func TestDetectEvents_EnemyBaseDiscovered(t *testing.T) {
	gs := baseGameState(100)
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Add building-based enemy intel
	gs.Tick = 101
	memory["enemyBases"] = map[string]rules.EnemyBaseIntel{
		"BadGuy": {Owner: "BadGuy", X: 100, Y: 200, Tick: 101, FromBuildings: true},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventEnemyBaseDiscovered {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected enemy_base_discovered event, got %+v", events)
	}
}

func TestDetectEvents_EnemyBaseDiscovered_UnitOnly(t *testing.T) {
	gs := baseGameState(100)
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Add unit-only enemy intel (not FromBuildings) — should NOT trigger
	gs.Tick = 101
	memory["enemyBases"] = map[string]rules.EnemyBaseIntel{
		"BadGuy": {Owner: "BadGuy", X: 100, Y: 200, Tick: 101, FromBuildings: false},
	}

	events := detectEvents(gs, memory, &prev)
	for _, e := range events {
		if e.Kind == EventEnemyBaseDiscovered {
			t.Errorf("did not expect enemy_base_discovered for unit-only intel, got %+v", events)
		}
	}
}

func TestDetectEvents_PhaseTransition(t *testing.T) {
	// Early → Mid triggers when a war factory appears (building milestone).
	gs := baseGameState(500)
	// Remove war factory so we start in Early Game.
	gs.Buildings = []model.Building{
		{ID: 1, Type: "fact", HP: 1000, MaxHP: 1000},
		{ID: 2, Type: "powr", HP: 400, MaxHP: 400},
		{ID: 3, Type: "proc", HP: 600, MaxHP: 600},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Add war factory → crosses into mid game.
	gs.Tick = 501
	gs.Buildings = append(gs.Buildings, model.Building{ID: 4, Type: "weap", HP: 800, MaxHP: 800})

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventPhaseTransition {
			found = true
			if e.Detail != "Phase transition: Early Game → Mid Game" {
				t.Errorf("unexpected detail: %s", e.Detail)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected phase_transition event, got %+v", events)
	}
}

func TestDetectEvents_PhaseTransition_MidToLate(t *testing.T) {
	// Mid → Late triggers when a tech center appears (building milestone).
	gs := baseGameState(1500)
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Add Soviet tech center → crosses into late game.
	gs.Tick = 1501
	gs.Buildings = append(gs.Buildings, model.Building{ID: 5, Type: "stek", HP: 600, MaxHP: 600})

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventPhaseTransition {
			found = true
			if e.Detail != "Phase transition: Mid Game → Late Game" {
				t.Errorf("unexpected detail: %s", e.Detail)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected phase_transition event, got %+v", events)
	}
}

func TestDetectEvents_PhaseTransition_TickFallback(t *testing.T) {
	// Even without milestone buildings, tick fallback triggers mid game at 2000.
	gs := model.GameState{
		Tick:   1999,
		Player: model.Player{Cash: 500, Resources: 500},
		Buildings: []model.Building{
			{ID: 1, Type: "fact"},
			{ID: 2, Type: "powr"},
		},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	gs.Tick = 2001

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventPhaseTransition {
			found = true
			if e.Detail != "Phase transition: Early Game → Mid Game" {
				t.Errorf("unexpected detail: %s", e.Detail)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected phase_transition from tick fallback, got %+v", events)
	}
}

func TestDetectEvents_PhaseTransition_BarracksWithArmy(t *testing.T) {
	// Barracks + 5 combat units should trigger Mid Game even without war factory.
	gs := model.GameState{
		Tick:   500,
		Player: model.Player{Cash: 500, Resources: 500},
		Buildings: []model.Building{
			{ID: 1, Type: "fact"},
			{ID: 2, Type: "powr"},
			{ID: 3, Type: "tent"}, // Allied barracks
		},
		Units: []model.Unit{
			{ID: 10, Type: "harv"},
			{ID: 11, Type: "e1"}, {ID: 12, Type: "e1"},
			{ID: 13, Type: "e1"}, {ID: 14, Type: "e1"},
		},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// 5th combat unit appears → crosses into mid game.
	gs.Tick = 501
	gs.Units = append(gs.Units, model.Unit{ID: 15, Type: "e1"})

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventPhaseTransition {
			found = true
			if e.Detail != "Phase transition: Early Game → Mid Game" {
				t.Errorf("unexpected detail: %s", e.Detail)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected phase_transition for barracks+army, got %+v", events)
	}
}

func TestDetectEvents_PhaseTransition_BarracksFewTroops(t *testing.T) {
	// Barracks + only 3 combat units should stay Early Game.
	gs := model.GameState{
		Tick:   500,
		Player: model.Player{Cash: 500, Resources: 500},
		Buildings: []model.Building{
			{ID: 1, Type: "fact"},
			{ID: 2, Type: "powr"},
			{ID: 3, Type: "barr"}, // Soviet barracks
		},
		Units: []model.Unit{
			{ID: 10, Type: "harv"},
			{ID: 11, Type: "e1"}, {ID: 12, Type: "e1"}, {ID: 13, Type: "e1"},
		},
	}
	phase := gamePhase(gs)
	if phase != "Early Game" {
		t.Errorf("expected Early Game with barracks + 3 troops, got %q", phase)
	}
}

func TestDetectEvents_EconomyCrisis_HarvestersLost(t *testing.T) {
	gs := baseGameState(100)
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Remove harvester
	gs.Tick = 101
	gs.Units = gs.Units[1:] // remove harv (id 10)

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventEconomyCrisis {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected economy_crisis event (harvesters lost), got %+v", events)
	}
}

func TestDetectEvents_EconomyCrisis_CashCollapse(t *testing.T) {
	gs := baseGameState(100)
	gs.Player.Cash = 800
	gs.Player.Resources = 300 // total 1100
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Cash collapses
	gs.Tick = 101
	gs.Player.Cash = 50
	gs.Player.Resources = 100 // total 150

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventEconomyCrisis {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected economy_crisis event (cash collapse), got %+v", events)
	}
}

func TestDetectEvents_SuperweaponReady(t *testing.T) {
	gs := baseGameState(100)
	gs.SupportPowers = []model.SupportPower{
		{Key: "NukeReady", Ready: false, RemainingTicks: 100, TotalTicks: 1000},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Nuke becomes ready
	gs.Tick = 101
	gs.SupportPowers[0].Ready = true
	gs.SupportPowers[0].RemainingTicks = 0

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventSuperweaponReady {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected superweapon_ready event, got %+v", events)
	}
}

func TestDetectEvents_FirstContact(t *testing.T) {
	gs := baseGameState(100)
	gs.Enemies = nil
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Enemies appear
	gs.Tick = 101
	gs.Enemies = []model.Enemy{
		{ID: 99, Owner: "BadGuy", Type: "3tnk", X: 200, Y: 300},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventFirstContact {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected first_contact event, got %+v", events)
	}
}

func TestDetectEvents_FirstContact_AlreadySeen(t *testing.T) {
	gs := baseGameState(100)
	gs.Enemies = []model.Enemy{
		{ID: 99, Owner: "BadGuy", Type: "3tnk", X: 200, Y: 300},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// More enemies, but not first contact
	gs.Tick = 101
	gs.Enemies = append(gs.Enemies, model.Enemy{ID: 100, Owner: "BadGuy", Type: "e1", X: 210, Y: 310})

	events := detectEvents(gs, memory, &prev)
	for _, e := range events {
		if e.Kind == EventFirstContact {
			t.Errorf("did not expect first_contact when enemies already seen, got %+v", events)
		}
	}
}

func TestFormatEvents_Empty(t *testing.T) {
	result := formatEvents(nil)
	if result != "" {
		t.Errorf("expected empty string for nil events, got %q", result)
	}
}

func TestFormatEvents_MultipleEvents(t *testing.T) {
	events := []Event{
		{Kind: EventCriticalBuildingLost, Tick: 500, Detail: "Lost critical building: fact (id 1)"},
		{Kind: EventArmyDevastated, Tick: 500, Detail: "Army devastated: 10→4 combat units (lost 60%)"},
	}
	result := formatEvents(events)
	if result == "" {
		t.Fatal("expected non-empty formatted events")
	}
	if !contains(result, "Recent Events:") {
		t.Error("missing 'Recent Events:' header")
	}
	if !contains(result, "critical_building_lost") {
		t.Error("missing critical_building_lost in output")
	}
	if !contains(result, "army_devastated") {
		t.Error("missing army_devastated in output")
	}
}

// --- strategy_countered tests ---

// infantryHeavyState returns a game state with many infantry for counter tests.
func infantryHeavyState(tick int) model.GameState {
	gs := model.GameState{
		Tick:   tick,
		Player: model.Player{Cash: 500, Resources: 500},
		Buildings: []model.Building{
			{ID: 1, Type: "fact"},
		},
		Units: []model.Unit{
			{ID: 10, Type: "harv"},
			{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
			{ID: 23, Type: "e3"}, {ID: 24, Type: "e3"}, {ID: 25, Type: "e1"},
			{ID: 26, Type: "e1"}, {ID: 27, Type: "e1"},
		},
	}
	return gs
}

func TestDetectEvents_StrategyCountered_InfantryVsFlame(t *testing.T) {
	gs := infantryHeavyState(100)
	// Enemy has flame towers
	gs.Enemies = []model.Enemy{
		{ID: 90, Owner: "BadGuy", Type: "ftur", X: 200, Y: 200},
		{ID: 91, Owner: "BadGuy", Type: "ftur", X: 210, Y: 200},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill 4 infantry
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
		{ID: 23, Type: "e3"},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			found = true
			if !contains(e.Detail, "infantry") {
				t.Errorf("expected 'infantry' in detail, got: %s", e.Detail)
			}
			if !contains(e.Detail, "Flame Tower") {
				t.Errorf("expected 'Flame Tower' in detail, got: %s", e.Detail)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected strategy_countered event, got %+v", events)
	}
}

func TestDetectEvents_StrategyCountered_InfantryVsTesla(t *testing.T) {
	gs := infantryHeavyState(100)
	gs.Enemies = []model.Enemy{
		{ID: 90, Owner: "BadGuy", Type: "tsla", X: 200, Y: 200},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill 3 infantry
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
		{ID: 23, Type: "e3"}, {ID: 24, Type: "e3"},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			found = true
			if !contains(e.Detail, "Tesla Coil") {
				t.Errorf("expected 'Tesla Coil' in detail, got: %s", e.Detail)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected strategy_countered event vs tesla, got %+v", events)
	}
}

func TestDetectEvents_StrategyCountered_VehiclesVsTesla(t *testing.T) {
	gs := model.GameState{
		Tick:   100,
		Player: model.Player{Cash: 500, Resources: 500},
		Buildings: []model.Building{{ID: 1, Type: "fact"}},
		Units: []model.Unit{
			{ID: 10, Type: "harv"},
			{ID: 30, Type: "3tnk"}, {ID: 31, Type: "3tnk"}, {ID: 32, Type: "3tnk"},
			{ID: 33, Type: "2tnk"}, {ID: 34, Type: "2tnk"}, {ID: 35, Type: "2tnk"},
		},
		Enemies: []model.Enemy{
			{ID: 90, Owner: "BadGuy", Type: "tsla"},
			{ID: 91, Owner: "BadGuy", Type: "tsla"},
		},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill 4 vehicles
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 30, Type: "3tnk"}, {ID: 31, Type: "3tnk"},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			found = true
			if !contains(e.Detail, "vehicle") {
				t.Errorf("expected 'vehicle' in detail, got: %s", e.Detail)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected strategy_countered event for vehicles, got %+v", events)
	}
}

func TestDetectEvents_StrategyCountered_AircraftVsSAM(t *testing.T) {
	gs := model.GameState{
		Tick:   100,
		Player: model.Player{Cash: 500, Resources: 500},
		Buildings: []model.Building{{ID: 1, Type: "fact"}},
		Units: []model.Unit{
			{ID: 40, Type: "heli"}, {ID: 41, Type: "heli"}, {ID: 42, Type: "heli"},
			{ID: 43, Type: "mig"}, {ID: 44, Type: "mig"},
		},
		Enemies: []model.Enemy{
			{ID: 90, Owner: "BadGuy", Type: "sam"},
			{ID: 91, Owner: "BadGuy", Type: "agun"},
		},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill 3 aircraft
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 40, Type: "heli"}, {ID: 41, Type: "heli"},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			found = true
			if !contains(e.Detail, "aircraft") {
				t.Errorf("expected 'aircraft' in detail, got: %s", e.Detail)
			}
			if !contains(e.Detail, "SAM Site") && !contains(e.Detail, "AA Gun") {
				t.Errorf("expected SAM/AA threat in detail, got: %s", e.Detail)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected strategy_countered event for aircraft, got %+v", events)
	}
}

func TestDetectEvents_StrategyCountered_InfantryAtThreshold(t *testing.T) {
	// Infantry threshold is 2, so losing 2 infantry SHOULD fire.
	gs := infantryHeavyState(100)
	gs.Enemies = []model.Enemy{
		{ID: 90, Owner: "BadGuy", Type: "ftur"},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill 2 infantry — at infantry threshold of 2, should fire
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
		{ID: 23, Type: "e3"}, {ID: 24, Type: "e3"}, {ID: 25, Type: "e1"},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected strategy_countered for 2 infantry losses (threshold=2), got %+v", events)
	}
}

func TestDetectEvents_StrategyCountered_VehicleBelowThreshold(t *testing.T) {
	// Vehicle threshold is still 3, so losing 2 vehicles should NOT fire.
	gs := model.GameState{
		Tick:   100,
		Player: model.Player{Cash: 500, Resources: 500},
		Buildings: []model.Building{{ID: 1, Type: "fact"}},
		Units: []model.Unit{
			{ID: 10, Type: "harv"},
			{ID: 30, Type: "3tnk"}, {ID: 31, Type: "3tnk"}, {ID: 32, Type: "3tnk"},
			{ID: 33, Type: "2tnk"}, {ID: 34, Type: "2tnk"},
		},
		Enemies: []model.Enemy{
			{ID: 90, Owner: "BadGuy", Type: "tsla"},
		},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill 2 vehicles — below vehicle threshold of 3
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 30, Type: "3tnk"}, {ID: 31, Type: "3tnk"}, {ID: 32, Type: "3tnk"},
	}

	events := detectEvents(gs, memory, &prev)
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			t.Errorf("did not expect strategy_countered for 2 vehicle losses (threshold=3), got %+v", events)
		}
	}
}

func TestDetectEvents_StrategyCountered_NoThreatsVisible(t *testing.T) {
	gs := infantryHeavyState(100)
	// No enemies visible — losses without visible counter-threats shouldn't trigger
	gs.Enemies = nil
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill 4 infantry
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
		{ID: 23, Type: "e3"},
	}

	events := detectEvents(gs, memory, &prev)
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			t.Errorf("did not expect strategy_countered without visible threats, got %+v", events)
		}
	}
}

func TestDetectEvents_StrategyCountered_Cooldown(t *testing.T) {
	gs := infantryHeavyState(100)
	gs.Enemies = []model.Enemy{
		{ID: 90, Owner: "BadGuy", Type: "ftur"},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)
	// Simulate a recent counter event
	prev.lastCounterTick = 50 // 100 - 50 = 50 ticks ago, below 200 cooldown

	// Kill 4 infantry
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
		{ID: 23, Type: "e3"},
	}

	events := detectEvents(gs, memory, &prev)
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			t.Errorf("did not expect strategy_countered during cooldown, got %+v", events)
		}
	}
}

func TestDetectEvents_StrategyCountered_CooldownExpired(t *testing.T) {
	gs := infantryHeavyState(500)
	gs.Enemies = []model.Enemy{
		{ID: 90, Owner: "BadGuy", Type: "ftur"},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)
	prev.lastCounterTick = 200 // 500 - 200 = 300 ticks ago, past 200 cooldown

	// Kill 4 infantry
	gs.Tick = 501
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
		{ID: 23, Type: "e3"},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected strategy_countered after cooldown expired, got %+v", events)
	}
}

func TestDetectEvents_StrategyCountered_EnemyFlamethrowerInfantry(t *testing.T) {
	gs := infantryHeavyState(100)
	// Enemy has flamethrower infantry (e4), not just static defenses
	gs.Enemies = []model.Enemy{
		{ID: 90, Owner: "BadGuy", Type: "e4", X: 200, Y: 200},
		{ID: 91, Owner: "BadGuy", Type: "e4", X: 210, Y: 200},
	}
	memory := make(map[string]any)
	prev := takeSnapshot(gs, memory)

	// Kill 4 infantry
	gs.Tick = 101
	gs.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
		{ID: 23, Type: "e3"},
	}

	events := detectEvents(gs, memory, &prev)
	found := false
	for _, e := range events {
		if e.Kind == EventStrategyCountered {
			found = true
			if !contains(e.Detail, "Flamethrower") {
				t.Errorf("expected 'Flamethrower' in detail, got: %s", e.Detail)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected strategy_countered vs enemy flamethrowers, got %+v", events)
	}
}

// --- Loss accumulation tests ---
// These test the mergeIDSets-based accumulation in UpdateState by simulating
// the same logic: carrying forward domain IDs across ticks so gradual losses
// (1 per tick) accumulate to reach the threshold.

func TestStrategyCountered_AccumulatesAcrossTicks(t *testing.T) {
	// Simulate 2 state updates where 1 infantry dies each time.
	// Without accumulation, each tick only sees 1 loss (below threshold of 2).
	// With accumulation, the losses add up and trigger on the 2nd update.

	memory := make(map[string]any)

	// Tick 100: 6 infantry alive, tesla coil visible
	gs0 := model.GameState{
		Tick:   100,
		Player: model.Player{Cash: 500, Resources: 500},
		Buildings: []model.Building{{ID: 1, Type: "fact"}},
		Units: []model.Unit{
			{ID: 10, Type: "harv"},
			{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
			{ID: 23, Type: "e1"}, {ID: 24, Type: "e1"}, {ID: 25, Type: "e1"},
		},
		Enemies: []model.Enemy{
			{ID: 90, Owner: "BadGuy", Type: "tsla"},
		},
	}

	// Simulate the strategist's accumulation loop.
	prev := takeSnapshot(gs0, memory)
	prev.lossBaselineTick = gs0.Tick

	// Tick 140: 1 infantry dies → accumulated loss = 1
	gs1 := gs0
	gs1.Tick = 140
	gs1.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
		{ID: 23, Type: "e1"}, {ID: 24, Type: "e1"},
		// ID 25 dead
	}
	events := detectEvents(gs1, memory, &prev)
	if hasEventKind(events, EventStrategyCountered) {
		t.Fatal("should not fire after only 1 loss")
	}
	// Accumulate: merge prev IDs + current alive IDs
	snap1 := takeSnapshot(gs1, memory)
	snap1.infantryIDs = mergeIDSets(prev.infantryIDs, snap1.infantryIDs)
	snap1.lossBaselineTick = prev.lossBaselineTick
	prev = snap1

	// Tick 180: another infantry dies → accumulated loss = 2, should fire! (infantry threshold=2)
	gs2 := gs1
	gs2.Tick = 180
	gs2.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
		{ID: 23, Type: "e1"},
		// ID 24, 25 dead
	}
	events = detectEvents(gs2, memory, &prev)
	if !hasEventKind(events, EventStrategyCountered) {
		t.Errorf("expected strategy_countered after 2 accumulated infantry losses (threshold=2), got %+v", events)
	}
}

func TestStrategyCountered_WindowResetsAfterCooldown(t *testing.T) {
	// After the accumulation window expires (counterCooldownTicks), the
	// baseline resets. Losses from before the window don't count.
	memory := make(map[string]any)

	gs0 := model.GameState{
		Tick:   100,
		Player: model.Player{Cash: 500, Resources: 500},
		Buildings: []model.Building{{ID: 1, Type: "fact"}},
		Units: []model.Unit{
			{ID: 10, Type: "harv"},
			{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
			{ID: 23, Type: "e1"}, {ID: 24, Type: "e1"},
		},
		Enemies: []model.Enemy{
			{ID: 90, Owner: "BadGuy", Type: "tsla"},
		},
	}

	prev := takeSnapshot(gs0, memory)
	prev.lossBaselineTick = gs0.Tick

	// Tick 140: 2 infantry die
	gs1 := gs0
	gs1.Tick = 140
	gs1.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"}, {ID: 22, Type: "e1"},
	}
	_ = detectEvents(gs1, memory, &prev)
	snap1 := takeSnapshot(gs1, memory)
	snap1.infantryIDs = mergeIDSets(prev.infantryIDs, snap1.infantryIDs)
	snap1.lossBaselineTick = prev.lossBaselineTick
	prev = snap1

	// Tick 350: window expired (350-100 = 250 > 200). Baseline should reset.
	// 1 more infantry dies, but baseline is fresh so only 1 loss counted.
	gs2 := gs1
	gs2.Tick = 350
	gs2.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"}, {ID: 21, Type: "e1"},
	}
	// Simulate window expiry: reset baseline (don't merge)
	snap2 := takeSnapshot(gs2, memory)
	snap2.lossBaselineTick = gs2.Tick // reset
	prev = snap2

	// Tick 390: another dies — only 1 since reset, should NOT fire
	gs3 := gs2
	gs3.Tick = 390
	gs3.Units = []model.Unit{
		{ID: 10, Type: "harv"},
		{ID: 20, Type: "e1"},
	}
	snap3 := takeSnapshot(gs3, memory)
	snap3.infantryIDs = mergeIDSets(prev.infantryIDs, snap3.infantryIDs)
	snap3.lossBaselineTick = prev.lossBaselineTick

	events := detectEvents(gs3, memory, &prev)
	if hasEventKind(events, EventStrategyCountered) {
		t.Errorf("should not fire — window reset, only 1 loss since reset, got %+v", events)
	}
}

func TestMergeIDSets(t *testing.T) {
	a := map[int]bool{1: true, 2: true, 3: true}
	b := map[int]bool{3: true, 4: true, 5: true}
	merged := mergeIDSets(a, b)
	if len(merged) != 5 {
		t.Errorf("expected 5 IDs in union, got %d", len(merged))
	}
	for _, id := range []int{1, 2, 3, 4, 5} {
		if !merged[id] {
			t.Errorf("expected id %d in merged set", id)
		}
	}
}

func hasEventKind(events []Event, kind EventKind) bool {
	for _, e := range events {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && containsStr(s, sub)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
