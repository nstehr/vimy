package rules

import (
	"slices"
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func assaultEnv() RuleEnv {
	return RuleEnv{
		Memory: map[string]any{
			"squads": map[string]*Squad{"s": {UnitIDs: []int{1, 2, 3, 4}}},
		},
		State: model.GameState{Units: []model.Unit{
			{ID: 1, Type: "2tnk", Idle: true},
			{ID: 2, Type: "2tnk"}, // moving
			{ID: 3, Type: "e1"},   // fighting
			{ID: 4, Type: "e1", Idle: true},
		}},
	}
}

func sorted(ids []uint32) []uint32 { slices.Sort(ids); return ids }

// The defect this exists to fix: Idle means "has no current order", so a squad
// executing its order does not satisfy it. Game 135 measured 1.3 of 4.0
// members reachable across 763 rallies, and the squad was then judged by
// SquadClumped, which counts all four.
func TestAssaultCommandsMovingMembersToo(t *testing.T) {
	env := assaultEnv()

	idle := squadIdleActorIDs(env, "s")
	if len(idle) != 2 {
		t.Fatalf("idle set = %v, want the 2 units with no order", idle)
	}

	all := squadAssaultActorIDs(env, "s")
	if got := sorted(all); len(got) != 4 {
		t.Errorf("assault set = %v, want all 4 members regardless of orders", got)
	}
}

// Retreating units must stay excluded: an assault that commands the wounded
// back into the fight defeats the retreat.
func TestAssaultSkipsRetreatingMembers(t *testing.T) {
	env := assaultEnv()
	env.Memory["retreatingUnits"] = map[int]int{2: 1}

	got := squadAssaultActorIDs(env, "s")
	if slices.Contains(got, uint32(2)) {
		t.Errorf("assault set %v must not include the retreating unit 2", got)
	}
	if len(got) != 3 {
		t.Errorf("assault set = %v, want the other 3", got)
	}
}

// A unit inside a disengage hold must stay excluded, or disengaging means
// nothing — it would be re-committed on the next evaluation. This is the one
// filter squadCommittableActorIDs deliberately omits, which is why the assault
// path needs its own set rather than reusing that one.
func TestAssaultHonoursTheDisengageHold(t *testing.T) {
	env := assaultEnv()
	env.State.Tick = 100
	env.Memory["disengagedUntil"] = map[int]int{3: 500}

	got := squadAssaultActorIDs(env, "s")
	if slices.Contains(got, uint32(3)) {
		t.Errorf("assault set %v must not include unit 3, held until tick 500", got)
	}

	// Once the hold lapses it rejoins.
	env.State.Tick = 600
	got = squadAssaultActorIDs(env, "s")
	if !slices.Contains(got, uint32(3)) {
		t.Errorf("assault set %v should include unit 3 after the hold expired", got)
	}
}

// Widening the set is only safe because sendAttackMove skips actors already
// holding the same order. Without that, every evaluation would cancel the
// squad's own paths and it would stall mid-map.
func TestRepeatedOrdersAreThrottledPerActor(t *testing.T) {
	env := assaultEnv()
	ids := squadAssaultActorIDs(env, "s")

	if !attackMoveHasFreshTarget(env, ids, 10, 10) {
		t.Fatal("first order to a destination must be considered fresh")
	}
	state := memoryMap[int, attackMoveEntry](env.Memory, "attackMoveSent")
	for _, id := range ids {
		state[int(id)] = attackMoveEntry{Tick: env.State.Tick, X: 10, Y: 10}
	}
	if attackMoveHasFreshTarget(env, ids, 10, 10) {
		t.Error("re-issuing the same destination immediately must be throttled")
	}
	if !attackMoveHasFreshTarget(env, ids, 40, 40) {
		t.Error("a different destination must still be sent")
	}
}

// A base alarm must draw on the garrison before the offensive. NearBaseGround
// Units reaches 0.20 of the map diagonal, which covers the ground a squad
// rallies on, so this used to sweep up the assault itself: game 136 fired the
// emergency 311 times against the assault's 133.
func TestEmergencyDefencePrefersTheGarrison(t *testing.T) {
	env := RuleEnv{
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack":  {Name: "ground-attack", Role: "attack", UnitIDs: []int{1, 2}},
				"ground-defense": {Name: "ground-defense", Role: "defend", UnitIDs: []int{3}},
			},
		},
	}
	near := []model.Unit{
		{ID: 1, Type: "2tnk"}, {ID: 2, Type: "2tnk"}, // committed to the assault
		{ID: 3, Type: "e1"}, {ID: 4, Type: "e1"}, // garrison and unassigned
	}

	got := withoutAttackSquads(env, near)
	if len(got) != 2 {
		t.Fatalf("got %d units, want the 2 not in an attacking squad", len(got))
	}
	for _, u := range got {
		if u.ID == 1 || u.ID == 2 {
			t.Errorf("unit %d is committed to the assault and must not be recalled", u.ID)
		}
	}
}

// When the offensive IS all there is, a base under attack still has to be
// answered — losing the base loses the game whatever the squad was doing.
func TestEmergencyDefenceFallsBackToTheOffensive(t *testing.T) {
	env := RuleEnv{
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Role: "attack", UnitIDs: []int{1, 2}},
			},
		},
	}
	near := []model.Unit{{ID: 1, Type: "2tnk"}, {ID: 2, Type: "2tnk"}}

	// The helper returns nothing, and the caller is what falls back.
	if got := withoutAttackSquads(env, near); len(got) != 0 {
		t.Fatalf("got %d, want 0: every nearby unit is committed", len(got))
	}
}

// No squads at all must not filter everything away.
func TestEmergencyDefenceWithNoSquads(t *testing.T) {
	env := RuleEnv{Memory: map[string]any{}}
	near := []model.Unit{{ID: 1}, {ID: 2}}
	if got := withoutAttackSquads(env, near); len(got) != 2 {
		t.Errorf("got %d, want both units when no squad exists", len(got))
	}
}

// The harvester scramble was the last path that could poach a committed
// assault. It pulls the NEAREST units by design, and a squad marching out
// across its own territory is the nearest force to a raided harvester — it
// matched 1320 times in game 144 and took units for 400 ticks each, which is
// most of the third of itself a squad lost between setting out and arriving.
func TestHarvesterScramblePrefersUncommittedUnits(t *testing.T) {
	env := RuleEnv{
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack":  {Name: "ground-attack", Role: "attack", UnitIDs: []int{1, 2}},
				"ground-defense": {Name: "ground-defense", Role: "defend", UnitIDs: []int{3}},
			},
		},
	}
	pool := []model.Unit{
		{ID: 1, Type: "2tnk"}, {ID: 2, Type: "2tnk"}, // marching on the enemy
		{ID: 3, Type: "e1"}, {ID: 4, Type: "e1"}, // garrison and spare
	}

	got := withoutAttackSquads(env, pool)
	if len(got) != 2 {
		t.Fatalf("got %d units, want the 2 not on the offensive", len(got))
	}
	for _, u := range got {
		if u.ID == 1 || u.ID == 2 {
			t.Errorf("unit %d is marching on the enemy base and must not be scrambled", u.ID)
		}
	}
}

// But a raid still has to be answered when the offensive is all there is —
// harvesters pay for the army.
func TestHarvesterScrambleFallsBackToTheOffensive(t *testing.T) {
	env := RuleEnv{
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Role: "attack", UnitIDs: []int{1, 2}},
			},
		},
	}
	pool := []model.Unit{{ID: 1, Type: "2tnk"}, {ID: 2, Type: "2tnk"}}
	if got := withoutAttackSquads(env, pool); len(got) != 0 {
		t.Fatalf("got %d, want 0 so the caller falls back to the whole pool", len(got))
	}
}
