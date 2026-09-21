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

// A committed squad stops recruiting.
//
// Game 155 rallied 22 times and struck zero. The rally itself worked: between
// recruitments the spread fell monotonically, 68 through 43, as the squad
// gathered. But each time it neared the gate a unit fresh off the war factory
// joined from the far side of the map and the spread reset. near plateaued at
// 9 while need climbed 9, 10, 10, 11 with every recruit — a tight core of nine
// and a tail that was always brand new. Recruitment outran convergence, so the
// gate could not pass no matter how well the squad marched.
func TestCommittedSquadStopsRecruiting(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Four in the squad, four more idle and unassigned at home.
	units := []model.Unit{
		{ID: 1, Type: "2tnk", X: 100, Y: 100, Idle: true},
		{ID: 2, Type: "2tnk", X: 101, Y: 100, Idle: true},
		{ID: 3, Type: "2tnk", X: 100, Y: 101, Idle: true},
		{ID: 4, Type: "2tnk", X: 102, Y: 102, Idle: true},
		{ID: 5, Type: "2tnk", X: 10, Y: 10, Idle: true},
		{ID: 6, Type: "2tnk", X: 11, Y: 10, Idle: true},
		{ID: 7, Type: "2tnk", X: 10, Y: 11, Idle: true},
		{ID: 8, Type: "2tnk", X: 11, Y: 11, Idle: true},
	}
	mem := map[string]any{
		"squads": map[string]*Squad{
			"ground-attack": {Name: "ground-attack", Domain: "ground", Role: "attack",
				UnitIDs: []int{1, 2, 3, 4}, TargetSize: 4},
		},
	}
	env := RuleEnv{
		State:  model.GameState{Tick: 2000, MapWidth: 128, MapHeight: 128, Units: units},
		Memory: mem,
	}

	// Mustering: the four at home are fair game.
	if err := FormSquad("ground-attack", "ground", 4, "attack")(env, conn); err != nil {
		t.Fatalf("muster: %v", err)
	}
	mustered := len(getSquads(mem)["ground-attack"].UnitIDs)
	if mustered <= 4 {
		t.Fatalf("squad = %d members, want it to grow while still mustering", mustered)
	}

	// Now commit it, which is what issuing an attack or rally order means.
	memoryMap[string, squadAttackState](mem, "squadAttackState")["ground-attack"] =
		squadAttackState{TargetX: 10, TargetY: 10, Attacking: true, LastTick: 2000}

	// A fresh tank rolls off the line, a map away from the squad.
	env.State.Units = append(units, model.Unit{ID: 9, Type: "2tnk", X: 5, Y: 5, Idle: true})
	if err := FormSquad("ground-attack", "ground", 4, "attack")(env, conn); err != nil {
		t.Fatalf("committed: %v", err)
	}
	if got := len(getSquads(mem)["ground-attack"].UnitIDs); got != mustered {
		t.Errorf("squad = %d members, want %d: a committed squad must not absorb new production", got, mustered)
	}

	// And a squad that re-forms from nothing is a new wave: recruitment reopens.
	delete(getSquads(mem), "ground-attack")
	if err := FormSquad("ground-attack", "ground", 4, "attack")(env, conn); err != nil {
		t.Fatalf("re-form: %v", err)
	}
	if squadCommitted(env, "ground-attack") {
		t.Error("re-forming must clear the old squad's commitment, or every later wave is frozen at birth")
	}
}
