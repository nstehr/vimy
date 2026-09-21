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

// A squad recruits from the units standing with it, and only those.
//
// Game 155 rallied 22 times without a strike. The rally worked — spread fell
// 68, 58, 54, 50, 46, 44, 43 as the squad gathered — but each time it neared
// the gate a unit fresh off the war factory joined from across the map and the
// spread reset. near plateaued at 9 while need climbed with every recruit.
//
// Refusing to recruit once committed was tried first and was too blunt. This
// design forms small and tops up on purpose, because unassigned-idle-ground
// runs a median of 0 and a maximum of 6, so a squad frozen at formation never
// grows past the two or three it started with: game 157 fielded squads
// averaging 4.3 members and mostly exactly 2. Position was always the real
// question, not commitment.
func TestSquadRecruitsOnlyFromUnitsStandingWithIt(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Four in the squad around (100,100), two more idle beside them, and two
	// far away at the factory.
	units := []model.Unit{
		{ID: 1, Type: "2tnk", X: 100, Y: 100, Idle: true},
		{ID: 2, Type: "2tnk", X: 101, Y: 100, Idle: true},
		{ID: 3, Type: "2tnk", X: 100, Y: 101, Idle: true},
		{ID: 4, Type: "2tnk", X: 102, Y: 102, Idle: true},
		{ID: 5, Type: "2tnk", X: 104, Y: 103, Idle: true},
		{ID: 6, Type: "2tnk", X: 103, Y: 104, Idle: true},
		{ID: 7, Type: "2tnk", X: 10, Y: 10, Idle: true},
		{ID: 8, Type: "2tnk", X: 11, Y: 11, Idle: true},
	}
	mem := map[string]any{
		"squads": map[string]*Squad{
			"ground-attack": {Name: "ground-attack", Domain: "ground", Role: "attack",
				UnitIDs: []int{1, 2, 3, 4}, TargetSize: 8},
		},
	}
	env := RuleEnv{
		State:  model.GameState{Tick: 2000, MapWidth: 128, MapHeight: 128, Units: units},
		Memory: mem,
	}
	if err := FormSquad("ground-attack", "ground", 8, "attack")(env, conn); err != nil {
		t.Fatalf("reinforce: %v", err)
	}

	got := map[int]bool{}
	for _, id := range getSquads(mem)["ground-attack"].UnitIDs {
		got[id] = true
	}
	// The two beside the squad must join: mustering has to work, or squads
	// never grow past the handful they form with.
	for _, id := range []int{5, 6} {
		if !got[id] {
			t.Errorf("unit %d stands with the squad and did not join", id)
		}
	}
	// The two at the far end must not: that is the join that wrecks formation.
	for _, id := range []int{7, 8} {
		if got[id] {
			t.Errorf("unit %d joined from %d cells away and resets the squad's spread", id, 127)
		}
	}
}
