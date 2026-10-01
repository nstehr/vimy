package agent

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// Vimy wrote a complete 15-parameter strategy every ~750 ticks — 53 times in
// game 139, 297 in game 135 — because ANY event could force one on a 100-tick
// cooldown. Two of them are the weather: harvester_under_attack fired 68 times
// in game 139 and first_contact 44. The rules already answer those.
func TestWeatherDoesNotTriggerAReplan(t *testing.T) {
	weather := []Event{
		{Kind: EventHarvesterUnderAttack},
		{Kind: EventFirstContact},
		{Kind: EventHarvesterUnderAttack},
	}
	if replanWorthy(weather) {
		t.Error("harassment and contact must not force a new grand strategy")
	}
	if replanWorthy(nil) {
		t.Error("no events must not trigger a replan")
	}
}

// What a human would actually stop and re-think for.
func TestRealSignalsTriggerAReplan(t *testing.T) {
	for _, kind := range []EventKind{
		EventStrategyCountered, EventCriticalBuildingLost, EventArmyDevastated,
		EventEconomyCrisis, EventEnemyBaseDiscovered, EventPhaseTransition,
		EventSuperweaponReady, EventHarvesterLost,
	} {
		if !replanWorthy([]Event{{Kind: kind}}) {
			t.Errorf("%s should trigger a replan", kind)
		}
	}
}

// One real signal buried in noise still counts — the filter drops weather, it
// does not require the batch to be clean.
func TestOneRealSignalAmongWeatherCounts(t *testing.T) {
	mixed := []Event{
		{Kind: EventHarvesterUnderAttack},
		{Kind: EventFirstContact},
		{Kind: EventCriticalBuildingLost},
	}
	if !replanWorthy(mixed) {
		t.Error("a critical building loss must count even alongside harassment")
	}
}

// A massing enemy is a rush before it has cost anything.
//
// Every other trigger in computePressureFlags is damage already taken, so the
// flag could only catch a rush that had both landed and landed early. Game 203
// saw enemy units near its base go from 2 to fifteen at tick 8000 and lost
// nothing until 11301 -- past rushTickCutoff -- so is-rushed() was false all
// game and build-base-defense-rush fired 0 times in 2559 evaluations.
func TestRushFlagSeesEnemiesMassingBeforeAnyLoss(t *testing.T) {
	base := []model.Building{{ID: 1, Type: "fact", X: 50, Y: 50}, {ID: 2, Type: "proc", X: 54, Y: 50}}
	near := func(n int) []model.Enemy {
		var out []model.Enemy
		for i := 0; i < n; i++ {
			out = append(out, model.Enemy{ID: 100 + i, Type: "e1", X: 56 + i, Y: 52})
		}
		return out
	}
	gs := func(enemies []model.Enemy) *model.GameState {
		return &model.GameState{MapWidth: 128, MapHeight: 128, Buildings: base, Enemies: enemies}
	}

	// No damage taken, no stress events -- exactly game 203 at tick 8000.
	if rushed, _ := computePressureFlags(8000, gs(near(rushEnemyNearBase)), nil); !rushed {
		t.Error("six enemy combat units beside the base with nothing lost yet did not read as a rush")
	}
	// A scouting pair must not.
	if rushed, _ := computePressureFlags(8000, gs(near(2)), nil); rushed {
		t.Error("two enemy units read as a rush; that is scouting")
	}
	// Past the cutoff the flag stays off however many are massing: by then it is
	// harassment or a real attack, and other rules own it.
	if rushed, _ := computePressureFlags(rushTickCutoff+1, gs(near(20)), nil); rushed {
		t.Error("the rush flag fired past rushTickCutoff")
	}
	// Harvesters are not combat units and must not count.
	var harv []model.Enemy
	for i := 0; i < 10; i++ {
		harv = append(harv, model.Enemy{ID: 200 + i, Type: "harv", X: 56, Y: 52})
	}
	if rushed, _ := computePressureFlags(8000, gs(harv), nil); rushed {
		t.Error("ten enemy harvesters read as a rush")
	}
	// A captured derrick is not part of the base we defend, so enemies beside one
	// far from home must not trigger it.
	far := &model.GameState{MapWidth: 128, MapHeight: 128,
		Buildings: []model.Building{{ID: 3, Type: "oilb", X: 10, Y: 10}},
		Enemies:   near(10)}
	if rushed, _ := computePressureFlags(8000, far, nil); rushed {
		t.Error("enemies near a captured derrick read as a rush on the base")
	}
}
