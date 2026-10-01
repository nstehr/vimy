package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// Recruits cross together when the crossing is long, and alone when it is short.
//
// The dispatch sent the whole pool, which looks like batching and is not:
// unassigned-idle-ground runs a median of 0 and units are produced one at a time,
// so game 205 measured 86 reinforcements of exactly 1.0 units each walking a mean
// 0.262 of the map diagonal. Game 202 lost 31 of 33 units that way, one or two at
// a time with the squad not in transit.
func TestReinforcementWaitsForAGroupOnLongCrossings(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Squad of 4 forward at (80,80); recruits at home (10,10) -- 99 cells, 0.77 of
	// a 128.7 diagonal, well past reinforceSoloFraction.
	build := func(nPool int, tick int) (RuleEnv, map[string]any) {
		units := []model.Unit{
			{ID: 1, Type: "e1", X: 80, Y: 80}, {ID: 2, Type: "e1", X: 80, Y: 80},
			{ID: 3, Type: "e1", X: 80, Y: 80}, {ID: 4, Type: "e1", X: 80, Y: 80},
		}
		for i := 0; i < nPool; i++ {
			units = append(units, model.Unit{ID: 10 + i, Type: "e1", X: 10, Y: 10, Idle: true})
		}
		mem := map[string]any{"squads": map[string]*Squad{
			"ground-attack": {Name: "ground-attack", Domain: "ground", Role: "attack",
				UnitIDs: []int{1, 2, 3, 4}, TargetSize: 12}}}
		return RuleEnv{State: model.GameState{Tick: tick, MapWidth: 91, MapHeight: 91, Units: units}, Memory: mem}, mem
	}

	// One recruit, long crossing: held, and the hold is recorded.
	env, mem := build(1, 1000)
	sink := &capture{}
	env.Events = sink
	if err := FormSquad("ground-attack", "ground", 12, "attack")(env, conn); err != nil {
		t.Fatal(err)
	}
	if n := len(getSquads(mem)["ground-attack"].Joining); n != 0 {
		t.Errorf("dispatched %d recruits alone across 0.77 of the map; should have waited", n)
	}
	if len(sink.events) != 1 || sink.events[0].Kind != "reinforce-held" {
		t.Fatalf("holding recruits emitted %v; a hold that is not recorded cannot be told from a leak", sink.events)
	}
	if got := sink.events[0].Idle; got != 1 {
		t.Errorf("pool waiting = %d, want 1", got)
	}
	if got := sink.events[0].Attrs["join_fraction"]; got < 0.74 || got > 0.80 {
		t.Errorf("join_fraction = %.3f, want ~0.769 (99 cells over a 128.7 diagonal)", got)
	}

	// Three recruits: enough to travel together.
	env3, mem3 := build(3, 1000)
	if err := FormSquad("ground-attack", "ground", 12, "attack")(env3, conn); err != nil {
		t.Fatal(err)
	}
	if n := len(getSquads(mem3)["ground-attack"].Joining); n != 3 {
		t.Errorf("dispatched %d of 3 recruits; a group should go", n)
	}

	// One recruit but the hold has expired: go anyway, or they stand at home
	// forever -- the failure squadCommitted exists to prevent.
	envH, memH := build(1, 1000)
	memH["reinforceHeldSince"] = map[string]int{"ground-attack": 1000 - reinforceMaxHold - 1}
	if err := FormSquad("ground-attack", "ground", 12, "attack")(envH, conn); err != nil {
		t.Fatal(err)
	}
	if n := len(getSquads(memH)["ground-attack"].Joining); n != 1 {
		t.Errorf("held %d recruits past the deadline; recruits that never dispatch stand at home", n)
	}

	// A SHORT crossing sends a single recruit immediately: squad at (12,12).
	envS := RuleEnv{State: model.GameState{Tick: 1000, MapWidth: 91, MapHeight: 91, Units: []model.Unit{
		{ID: 1, Type: "e1", X: 12, Y: 12}, {ID: 2, Type: "e1", X: 12, Y: 12},
		{ID: 10, Type: "e1", X: 10, Y: 10, Idle: true},
	}}, Memory: map[string]any{"squads": map[string]*Squad{
		"ground-attack": {Name: "ground-attack", Domain: "ground", Role: "attack", UnitIDs: []int{1, 2}, TargetSize: 12}}}}
	if err := FormSquad("ground-attack", "ground", 12, "attack")(envS, conn); err != nil {
		t.Fatal(err)
	}
	if n := len(getSquads(envS.Memory)["ground-attack"].Joining); n != 1 {
		t.Errorf("held a recruit joining a squad beside it; a short walk costs nothing")
	}
}
