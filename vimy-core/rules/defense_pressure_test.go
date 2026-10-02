package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// pressureState puts our base at (20,20) and an enemy `away` cells from it. The map
// is 100x100, so the diagonal is ~141 and base-under-attack() trips inside 28 cells.
// damagedHP below MaxHP is what makes a building count as damaged.
func pressureState(tick, away int, damaged bool, buildingsDead int) model.GameState {
	hp := 1000
	if damaged {
		hp = 400
	}
	return model.GameState{
		Tick:      tick,
		MapWidth:  100,
		MapHeight: 100,
		Player:    model.Player{Cash: 500, PowerProvided: 200, PowerDrained: 50, BuildingsDead: buildingsDead},
		Buildings: []model.Building{{ID: 1, Type: "fact", X: 20, Y: 20, HP: hp, MaxHP: 1000}},
		Units:     []model.Unit{{ID: 10, Type: "e1", X: 22, Y: 22}},
		Enemies:   []model.Enemy{{ID: 99, Type: "3tnk", X: 20 + away, Y: 20, HP: 400, MaxHP: 400}},
	}
}

// An episode records what the pressure cost and how many acts it absorbed.
//
// The point of the rows: defend-base acted 226-571 times in each of games 205-210
// and those games lost 37-74 buildings anyway. stream_rule_evals can say the rule
// fired; only this can say what the episode cost.
func TestDefensePressureRecordsCostAndResponse(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	env := func(tick, away int, damaged bool, dead int) RuleEnv {
		return RuleEnv{State: pressureState(tick, away, damaged, dead), Memory: mem, Events: sink}
	}

	// Enemy 5 cells out: well inside the 28-cell trip, and inside the 10-cell
	// range that makes it critical, with the building damaged.
	trackDefensePressure(env(1000, 5, true, 3))
	if len(sink.events) != 1 || sink.events[0].Kind != "base-pressure" {
		t.Fatalf("pressure at the base emitted %v", sink.events)
	}
	if got := sink.events[0].Reason; got != "inside" && got != "perimeter" {
		t.Errorf("threat depth = %q; an enemy 5 cells from a base centred at (20,20) is not %q", got, got)
	}

	// Two defend-base acts land during the episode, one from the garrison and one
	// that had to recall the assault.
	e := env(1100, 5, true, 3)
	recordDefendBase(e, "garrison", []model.Unit{{ID: 10, X: 22, Y: 22}, {ID: 11, X: 23, Y: 21}}, &model.Enemy{ID: 99, X: 25, Y: 20})
	trackDefensePressure(e)
	e2 := env(1200, 5, true, 4) // a building has died since
	recordDefendBase(e2, "assault recalled", []model.Unit{{ID: 12, X: 90, Y: 90}}, &model.Enemy{ID: 99, X: 25, Y: 20})
	trackDefensePressure(e2)

	for _, tick := range []int{1300, 1400} {
		trackDefensePressure(env(tick, 5, true, 4))
	}
	// Past the beat.
	trackDefensePressure(env(1500, 5, true, 4))

	beat := sink.events[len(sink.events)-1]
	if beat.Kind != "base-pressure-held" {
		t.Fatalf("last event = %q, want base-pressure-held", beat.Kind)
	}
	if beat.Attrs["acts"] != 2 {
		t.Errorf("acts = %v, want 2", beat.Attrs["acts"])
	}
	if beat.Attrs["tier_garrison"] != 1 || beat.Attrs["tier_assault"] != 1 {
		t.Errorf("tiers = garrison %v assault %v, want 1 and 1",
			beat.Attrs["tier_garrison"], beat.Attrs["tier_assault"])
	}
	// Buildings dead went 3 -> 4 during the episode. That is the cost column, and
	// it comes from the engine's own counter rather than from counting what stands.
	if beat.Attrs["buildings_lost"] != 1 {
		t.Errorf("buildings_lost = %v, want 1", beat.Attrs["buildings_lost"])
	}
	if beat.Attrs["n_damaged"] == 0 || beat.Attrs["n_critical"] == 0 {
		t.Errorf("n_damaged = %v, n_critical = %v; both must be non-zero for a real attack",
			beat.Attrs["n_damaged"], beat.Attrs["n_critical"])
	}
	// One responder came from the far corner, one pair from next door, so the mean
	// crossing is large enough to be visible but is not the whole map.
	if f := beat.Attrs["mean_response_fraction"]; f <= 0 || f >= 1 {
		t.Errorf("mean_response_fraction = %v, want a fraction of the diagonal", f)
	}
	if beat.Attrs["mean_responders"] != 1.5 {
		t.Errorf("mean_responders = %v, want 1.5 (2 then 1)", beat.Attrs["mean_responders"])
	}
}

// A loiterer is recorded as a loiterer: pressure with no damage and no losses.
//
// base-under-attack() is proximity, not damage -- any enemy within 20 percent of
// the diagonal. It is true in 27-44 percent of states in the losses, and if those
// episodes carry no damage then the finding is about the predicate, not about
// defence. These are the columns that tell the two apart, so they must not report
// damage where there is none.
func TestDefensePressureDistinguishesLoiteringFromDestruction(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	// 20 cells out: inside the 28-cell proximity trip, outside the 10-cell
	// critical range, and nothing damaged.
	for _, tick := range []int{1000, 1100, 1200, 1300, 1400, 1500} {
		trackDefensePressure(RuleEnv{State: pressureState(tick, 20, false, 0), Memory: mem, Events: sink})
	}
	beat := sink.events[len(sink.events)-1]
	if beat.Kind != "base-pressure-held" {
		t.Fatalf("last event = %q, want base-pressure-held", beat.Kind)
	}
	if beat.Attrs["n_damaged"] != 0 || beat.Attrs["n_critical"] != 0 {
		t.Errorf("n_damaged = %v, n_critical = %v; an enemy loitering at 20 cells is damaging nothing",
			beat.Attrs["n_damaged"], beat.Attrs["n_critical"])
	}
	if beat.Attrs["buildings_lost"] != 0 {
		t.Errorf("buildings_lost = %v, want 0", beat.Attrs["buildings_lost"])
	}
	if beat.Reason != "outskirts" {
		t.Errorf("threat depth = %q, want outskirts -- the band base-under-attack fires on harmlessly", beat.Reason)
	}
}

// The episode closes when the enemy leaves, and the state is forgotten.
func TestDefensePressureClosesWhenTheThreatLeaves(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	trackDefensePressure(RuleEnv{State: pressureState(1000, 5, true, 0), Memory: mem, Events: sink})
	// 60 cells out: past the 28-cell trip.
	trackDefensePressure(RuleEnv{State: pressureState(2000, 60, false, 0), Memory: mem, Events: sink})

	last := sink.events[len(sink.events)-1]
	if last.Kind != "base-pressure-cleared" {
		t.Fatalf("events = %v, want base-pressure-cleared", sink.events)
	}
	if last.Attrs["elapsed"] != 1000 {
		t.Errorf("elapsed = %v, want 1000", last.Attrs["elapsed"])
	}
	if _, still := mem[defensePressureKey]; still {
		t.Error("episode survived the threat leaving; the next alarm would report one long siege")
	}
}

// An act with no episode open is dropped rather than folded into a neighbour.
//
// defend-base requires the same predicates the tracker keys on, so this should be
// unreachable; if it ever happens it is a separate bug, and attributing the act to
// an adjacent episode would hide it.
func TestDefendBaseActOutsideAnEpisodeIsNotCounted(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	e := RuleEnv{State: pressureState(1000, 60, false, 0), Memory: mem, Events: sink}
	recordDefendBase(e, "garrison", []model.Unit{{ID: 10}}, &model.Enemy{ID: 7, X: 5, Y: 5})
	if len(sink.events) != 0 {
		t.Errorf("emitted %v for an act with no pressure episode", sink.events)
	}
	if _, created := mem[defensePressureKey]; created {
		t.Error("an act created an episode; only trackDefensePressure may open one")
	}
}

// Target churn is counted, because it is what decides whether re-ordering hurts.
//
// Game 211 measured 1687 defend-base acts across 1889 evaluations under pressure --
// an order roughly every 10 ticks, for the whole episode. Re-sending the SAME
// attack-move is near enough a no-op; a new target every few ticks is units
// re-pathing instead of shooting. Nothing recorded which, so game 211 could not
// settle it.
func TestDefensePressureCountsTargetChurn(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	trackDefensePressure(RuleEnv{State: pressureState(1000, 5, true, 0), Memory: mem, Events: sink})

	e := RuleEnv{State: pressureState(1100, 5, true, 0), Memory: mem, Events: sink}
	// Same target three times, then a different one, then back.
	for _, id := range []int{99, 99, 99, 42, 99} {
		recordDefendBase(e, "garrison", []model.Unit{{ID: 10, X: 22, Y: 22}}, &model.Enemy{ID: id, X: 25, Y: 20})
	}
	trackDefensePressure(e)
	trackDefensePressure(RuleEnv{State: pressureState(2000, 60, false, 0), Memory: mem, Events: sink})

	end := sink.events[len(sink.events)-1]
	if end.Kind != "base-pressure-cleared" {
		t.Fatalf("last event = %q", end.Kind)
	}
	if got := end.Attrs["acts"]; got != 5 {
		t.Errorf("acts = %v, want 5", got)
	}
	// 99,99,99,42,99: two switches. The first act sets the target and is not a change.
	if got := end.Attrs["n_target_changes"]; got != 2 {
		t.Errorf("n_target_changes = %v, want 2 -- re-sending the same order is not churn", got)
	}
}
