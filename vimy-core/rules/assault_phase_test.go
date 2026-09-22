package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// The four reasons a strike does not happen must stay distinguishable, because
// they have four different fixes and the phase log cannot tell them apart.
// Game 131 reached `strike` zero times in 34 transitions and the phase log
// could only say that it had been rallying, approaching and hunting.
func TestStrikeBlockersAreCountedSeparately(t *testing.T) {
	env := RuleEnv{Memory: map[string]any{}}
	recordStrikeBlocked(env, "ground-attack", StrikeBlockedUnclumped)
	recordStrikeBlocked(env, "ground-attack", StrikeBlockedUnclumped)
	recordStrikeBlocked(env, "ground-attack", StrikeBlockedOutOfReach)

	got := env.StrikeBlockers()
	if got[StrikeBlockedUnclumped] != 2 {
		t.Errorf("unclumped = %d, want 2", got[StrikeBlockedUnclumped])
	}
	if got[StrikeBlockedOutOfReach] != 1 {
		t.Errorf("out-of-reach = %d, want 1", got[StrikeBlockedOutOfReach])
	}
	// A reason that never happened must read as absent, not as zero recorded.
	if _, seen := got[StrikeBlockedBlindAtBase]; seen {
		t.Errorf("blind-at-base was never recorded and must not appear")
	}
}

// A rally is a gathering move for the walk in, and it expires on the doorstep.
//
// The blocked branch attack-moves the squad onto its OWN centroid. Far from
// the target that is a gathering move and correct. On the doorstep it is
// "stand still", re-issued every evaluation, inside the base's defensive
// envelope.
//
// So the gate must still hold a dispersed squad that is a walk away, and must
// not hold one that has already arrived. Note this is a guard against a rare
// case, not a fix for a common one: the justification originally given here
// ("27 units to within 17% of the enemy base") misread a transit row's `near`,
// which counts members inside the clump radius rather than measuring distance.
// Vimy's squads have not been reaching the enemy base at all.
func TestUnclumpedSquadRalliesFarOutButCommitsOnTheDoorstep(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Four together and two far behind: four near against a need of five, so
	// the clump gate fails in both cases below and only distance differs.
	units := []model.Unit{
		{ID: 1, X: 100, Y: 100}, {ID: 2, X: 101, Y: 100},
		{ID: 3, X: 100, Y: 101}, {ID: 4, X: 102, Y: 102},
		{ID: 5, X: 140, Y: 100}, {ID: 6, X: 160, Y: 100},
	}
	envAt := func(baseX, baseY int) RuleEnv {
		return RuleEnv{
			State: model.GameState{Tick: 1000, MapWidth: 128, MapHeight: 128, Units: units},
			Memory: map[string]any{
				"squads": map[string]*Squad{
					"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{1, 2, 3, 4, 5, 6}},
				},
				"enemyBases": map[string]EnemyBaseIntel{
					"red": {Owner: "red", X: baseX, Y: baseY, Tick: 1, FromBuildings: true},
				},
			},
		}
	}
	// Guard the premise: if this squad ever clumps, the test proves nothing.
	if envAt(10, 10).SquadClumped("ground-attack", squadRallyRadius) {
		t.Fatal("premise broken: the test squad is supposed to be dispersed")
	}

	// The centroid sits near x=117. A base at (10,10) is a long walk away.
	far := envAt(10, 10)
	if err := SquadAttackKnownBase("ground-attack", 1.0)(far, conn); err != nil {
		t.Fatalf("far assault: %v", err)
	}
	state := memoryMap[string, squadAttackState](far.Memory, "squadAttackState")
	if state["ground-attack"].Attacking {
		t.Error("a dispersed squad a map away should rally, not commit")
	}
	if got := far.StrikeBlockers()[StrikeBlockedUnclumped]; got != 1 {
		t.Errorf("unclumped blocks = %d, want 1 on the long walk", got)
	}

	// Same dispersion, but the base is now under the squad's feet.
	near := envAt(120, 100)
	if err := SquadAttackKnownBase("ground-attack", 1.0)(near, conn); err != nil {
		t.Fatalf("near assault: %v", err)
	}
	state = memoryMap[string, squadAttackState](near.Memory, "squadAttackState")
	if !state["ground-attack"].Attacking {
		t.Error("an arrived squad must commit: holding still under the base's guns is the grinder")
	}
	if got := near.StrikeBlockers()[StrikeBlockedUnclumped]; got != 0 {
		t.Errorf("unclumped blocks = %d, want 0 once arrived", got)
	}
}

// A squad takes reinforcements while it travels, and refuses them once it is
// in contact.
//
// Game 155's treadmill was a unit fresh off the war factory joining a squad at
// the enemy base, resetting spread to 50-odd each time the clump gate came
// within reach. A fixed join radius stopped that and caused the opposite
// failure: game 159 sat on 44 idle units with the squad frozen at 6, because
// everyone at home was out of range and a second squad could not form.
//
// So the line is contact, not distance. The joiner's walk is free; the
// reshuffle at the gate is not.
func TestSquadReinforcesWhileTravellingButNotInContact(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Four in the squad at (100,100), four idle recruits back at base.
	units := []model.Unit{
		{ID: 1, Type: "2tnk", X: 100, Y: 100, Idle: true},
		{ID: 2, Type: "2tnk", X: 101, Y: 100, Idle: true},
		{ID: 3, Type: "2tnk", X: 100, Y: 101, Idle: true},
		{ID: 4, Type: "2tnk", X: 102, Y: 102, Idle: true},
		{ID: 5, Type: "2tnk", X: 10, Y: 10, Idle: true},
		{ID: 6, Type: "2tnk", X: 11, Y: 11, Idle: true},
		{ID: 7, Type: "2tnk", X: 10, Y: 12, Idle: true},
		{ID: 8, Type: "2tnk", X: 12, Y: 10, Idle: true},
	}
	build := func(baseX, baseY int) (RuleEnv, map[string]any) {
		mem := map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Domain: "ground", Role: "attack",
					UnitIDs: []int{1, 2, 3, 4}, TargetSize: 8},
			},
			"enemyBases": map[string]EnemyBaseIntel{
				"red": {Owner: "red", X: baseX, Y: baseY, Tick: 1, FromBuildings: true},
			},
		}
		return RuleEnv{
			State:  model.GameState{Tick: 2000, MapWidth: 128, MapHeight: 128, Units: units},
			Memory: mem,
		}, mem
	}

	// Enemy base far away: the squad is travelling, so the recruits at home
	// must be able to join. Stranding them is how 44 units sat idle.
	env, mem := build(10, 10)
	if err := FormSquad("ground-attack", "ground", 8, "attack")(env, conn); err != nil {
		t.Fatalf("travelling: %v", err)
	}
	if got := len(getSquads(mem)["ground-attack"].UnitIDs); got <= 4 {
		t.Errorf("squad = %d members, want it to absorb recruits while travelling", got)
	}

	// Enemy base under the squad's feet: it is in contact and must not
	// reshuffle for anyone.
	env2, mem2 := build(101, 101)
	if !squadAssaulting(env2, "ground-attack") {
		t.Fatal("premise broken: the squad is supposed to be in contact")
	}
	if err := FormSquad("ground-attack", "ground", 8, "attack")(env2, conn); err != nil {
		t.Fatalf("in contact: %v", err)
	}
	if got := len(getSquads(mem2)["ground-attack"].UnitIDs); got != 4 {
		t.Errorf("squad = %d members, want 4: a squad at the gate must not take joiners", got)
	}
}

// squad-reengage nudges stragglers; it must not steer the army.
//
// It used squad-attack-move, the assault action, which commands EVERY member —
// so one idle straggler redirected the whole squad, and at bestTargetForSquad
// rather than the base the assault was marching on. The rule is category
// combat, not the exclusive ground-attack-choice, so it acted alongside the
// exclusive winner rather than competing with it: two rules steering the same
// units at two different targets every tick. Game 159 sawtoothed 51 units
// across 0.3 of the map diagonal in front of the enemy base for thousands of
// ticks without ever closing.
func TestNudgeMovesOnlyTheStragglers(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Four marching (under orders, so not idle) and two stragglers standing.
	units := []model.Unit{
		{ID: 1, Type: "2tnk", X: 100, Y: 100},
		{ID: 2, Type: "2tnk", X: 101, Y: 100},
		{ID: 3, Type: "2tnk", X: 100, Y: 101},
		{ID: 4, Type: "2tnk", X: 102, Y: 102},
		{ID: 5, Type: "2tnk", X: 60, Y: 60, Idle: true},
		{ID: 6, Type: "2tnk", X: 61, Y: 61, Idle: true},
	}
	mem := map[string]any{
		"squads": map[string]*Squad{
			"ground-attack": {Name: "ground-attack", Domain: "ground", Role: "attack",
				UnitIDs: []int{1, 2, 3, 4, 5, 6}, TargetSize: 6},
		},
	}
	env := RuleEnv{
		State:  model.GameState{Tick: 3000, MapWidth: 128, MapHeight: 128, Units: units},
		Memory: mem,
	}
	if err := SquadNudgeStragglers("ground-attack")(env, conn); err != nil {
		t.Fatalf("nudge: %v", err)
	}

	// sendAttackMove records who it ordered. Only the two idle ones may appear.
	sent := memoryMap[int, attackMoveEntry](mem, "attackMoveSent")
	for _, id := range []int{5, 6} {
		if _, ok := sent[id]; !ok {
			t.Errorf("straggler %d was not nudged", id)
		}
	}
	for _, id := range []int{1, 2, 3, 4} {
		if _, ok := sent[id]; ok {
			t.Errorf("marching unit %d was redirected: one straggler must not steer the army", id)
		}
	}
}
