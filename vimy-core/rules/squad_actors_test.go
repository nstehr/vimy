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

// The squad target must track the army, not freeze at what existed when it
// formed. ground_attack_group_size is a constant — 5 or 6 in every game the
// strategist has written — while the army runs from 7 combat units to 27 at
// peak, and TargetSize was set once at formation and never revisited. Game 150
// formed a full 6, held formation, reached 0.143 of the map diagonal, and
// arrived with two units.
func TestSquadTargetGrowsWithTheArmy(t *testing.T) {
	env := RuleEnv{Memory: map[string]any{}, State: model.GameState{}}

	// Four combat units and a doctrine asking for six: the floor wins.
	for i := 1; i <= 4; i++ {
		env.State.Units = append(env.State.Units, model.Unit{ID: i, Type: "2tnk"})
	}
	if got := squadTarget(env, "ground-attack", 6); got != 6 {
		t.Errorf("target = %d, want the doctrine floor of 6", got)
	}

	// The army grows to twenty. The target must follow.
	for i := 5; i <= 20; i++ {
		env.State.Units = append(env.State.Units, model.Unit{ID: i, Type: "e1"})
	}
	if got := squadTarget(env, "ground-attack", 6); got != 20 {
		t.Errorf("target = %d, want 20: the target must track the army", got)
	}
}

// The garrison is not available to the offensive. Units rostered to another
// squad are already spoken for — ground-defense absorbed 3604 defence acts on
// its own without the offensive being touched.
func TestSquadTargetExcludesOtherSquads(t *testing.T) {
	env := RuleEnv{
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-defense": {Name: "ground-defense", UnitIDs: []int{1, 2, 3}},
			},
		},
		State: model.GameState{Units: []model.Unit{
			{ID: 1, Type: "2tnk"}, {ID: 2, Type: "2tnk"}, {ID: 3, Type: "e1"},
			{ID: 4, Type: "e1"}, {ID: 5, Type: "e1"}, {ID: 6, Type: "arty"},
		}},
	}
	// Six combat units, three held by the garrison: three are committable, so
	// the floor of 4 wins.
	if got := squadTarget(env, "ground-attack", 4); got != 4 {
		t.Errorf("target = %d, want 4: only three units are uncommitted", got)
	}
	// The squad's OWN members still count toward its target.
	getSquads(env.Memory)["ground-attack"] = &Squad{Name: "ground-attack", UnitIDs: []int{4, 5}}
	if got := squadTarget(env, "ground-attack", 2); got != 3 {
		t.Errorf("target = %d, want 3: own members plus the unassigned one", got)
	}
}

// Harvesters and aircraft are not the ground offensive.
func TestSquadTargetCountsOnlyCombatGround(t *testing.T) {
	env := RuleEnv{
		Memory: map[string]any{},
		State: model.GameState{Units: []model.Unit{
			{ID: 1, Type: "2tnk"}, {ID: 2, Type: "e1"},
			{ID: 3, Type: "harv"}, {ID: 4, Type: "mig"}, {ID: 5, Type: "harv"},
		}},
	}
	if got := squadTarget(env, "ground-attack", 1); got != 2 {
		t.Errorf("target = %d, want 2: harvesters and aircraft are not the offensive", got)
	}
}

// Only the offensive absorbs the army. The garrison asks for a handful of
// units on purpose, and form-defense-squad runs at roughly 475 against the
// attack's 265 — so scaling it too meant it took everything first and left the
// assault a remnant: 35 units on the field and a squad of 6 at tick 13500.
func TestOnlyTheOffensiveScalesWithTheArmy(t *testing.T) {
	units := make([]model.Unit, 0, 12)
	for i := 1; i <= 12; i++ {
		units = append(units, model.Unit{ID: i, Type: "e1", Idle: true})
	}

	def := RuleEnv{Memory: map[string]any{}, State: model.GameState{Units: units}}
	if err := FormSquad("ground-defense", "ground", 3, "defend")(def, nil); err != nil {
		t.Fatalf("FormSquad: %v", err)
	}
	if got := getSquads(def.Memory)["ground-defense"]; got.TargetSize != 3 {
		t.Errorf("garrison target = %d, want the 3 it asked for", got.TargetSize)
	}

	atk := RuleEnv{Memory: map[string]any{}, State: model.GameState{Units: units}}
	if err := FormSquad("ground-attack", "ground", 3, "attack")(atk, nil); err != nil {
		t.Fatalf("FormSquad: %v", err)
	}
	if got := getSquads(atk.Memory)["ground-attack"]; got.TargetSize != 12 {
		t.Errorf("assault target = %d, want all 12 committable units", got.TargetSize)
	}
}
