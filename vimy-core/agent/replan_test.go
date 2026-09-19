package agent

import "testing"

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
