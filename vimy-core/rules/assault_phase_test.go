package rules

import (
	"math"
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

// The three ways of declining a detour must be distinguishable.
//
// BestApproachAxis returns false when it has no data, when the direct corridor
// scores as already open, and when no flank is meaningfully cleaner. From the
// call site all three look the same: the squad walks the direct line into the
// defences. Game 172 did exactly that while having observed one flame tower,
// which is the no-intel case wearing the already-open case's clothes.
func TestApproachChoiceIsRecordedWithItsReason(t *testing.T) {
	// No terrain: the no-data path.
	env := RuleEnv{Memory: map[string]any{}, State: model.GameState{MapWidth: 128, MapHeight: 128}}
	if _, _, ok := env.BestApproachAxis(50, 50); ok {
		t.Fatal("expected no detour without terrain")
	}
	if got := env.ApproachChoices()[ApproachNoData]; got != 1 {
		t.Errorf("no-data count = %d, want 1", got)
	}

	// Terrain present and nothing remembered: every corridor scores zero, which
	// is the case that has been silently reading as "the front door is fine".
	terrain := &model.TerrainGrid{Cols: 32, Rows: 32, CellW: 4, CellH: 4}
	open := RuleEnv{
		Memory:  map[string]any{},
		State:   model.GameState{MapWidth: 128, MapHeight: 128, Buildings: []model.Building{{ID: 1, X: 10, Y: 10}}},
		Terrain: terrain,
	}
	if _, _, ok := open.BestApproachAxis(100, 100); ok {
		t.Fatal("expected no detour with an empty threat field")
	}
	if got := open.ApproachChoices()[ApproachOpen]; got != 1 {
		t.Errorf("already-open count = %d, want 1 — this is the no-intel case and must be visible", got)
	}
	if got := open.ApproachChoices()[ApproachNoData]; got != 0 {
		t.Errorf("no-data leaked into a run that had terrain: %d", got)
	}
}

// Artillery holds at range; the rest of the squad closes.
//
// Every RA base defence is short-ranged - TurretGun 6c512 is the longest,
// TeslaZap 6c0, the flame tower 5c0 - and Allied artillery reaches 20c0. The
// assault sent the entire squad to the base centroid, so a 20-cell gun walked
// into a 5-cell flame tower beside the riflemen and died there. That threefold
// range advantage was simply unused.
func TestSiegeUnitsHoldAtStandoffWhileTheRestClose(t *testing.T) {
	for _, c := range []struct {
		unit string
		want int
	}{
		{"arty", 16}, // 20 - 4
		{"v2rl", 9},  // 10 - 4 = 6, floored at maxBaseDefenceRange + 2
		{"2tnk", 0},  // not a siege unit
	} {
		got, ok := siegeStandoffCells(c.unit)
		if c.want == 0 {
			if ok {
				t.Errorf("%s: treated as siege", c.unit)
			}
			continue
		}
		if !ok || got != c.want {
			t.Errorf("%s standoff = %d (ok=%v), want %d", c.unit, got, ok, c.want)
		}
		if got <= maxBaseDefenceRange {
			t.Errorf("%s holds at %d, inside the %d-cell reach of a base defence", c.unit, got, maxBaseDefenceRange)
		}
		if r := siegeRangeCells[c.unit]; got > r {
			t.Errorf("%s holds at %d, beyond its own %d-cell range", c.unit, got, r)
		}
	}

	// The standoff point sits on the line back toward the squad, at the right
	// distance from the target - including when the squad has already closed
	// too far, where it must pull them back out.
	sx, sy := standoffPoint(0, 0, 100, 0, 16)
	if sx != 84 || sy != 0 {
		t.Errorf("standoff = (%d,%d), want (84,0)", sx, sy)
	}
	sx, _ = standoffPoint(98, 0, 100, 0, 16)
	if sx != 84 {
		t.Errorf("standoff from inside = %d, want it pulled back out to 84", sx)
	}

	// And the split keeps the two groups apart.
	env := RuleEnv{State: model.GameState{Units: []model.Unit{
		{ID: 1, Type: "2tnk"}, {ID: 2, Type: "arty"}, {ID: 3, Type: "e1"}, {ID: 4, Type: "arty.husk"},
	}}}
	closers, siege := splitSiege(env, []uint32{1, 2, 3})
	if len(siege) != 1 {
		t.Fatalf("siege group = %d units, want 1", len(siege))
	}
	if _, ok := siege[2]; !ok {
		t.Error("the artillery was not held back")
	}
	if len(closers) != 2 {
		t.Errorf("closers = %d, want the tank and the rifleman", len(closers))
	}
}

// And the assault must actually issue the two orders separately.
//
// The helper tests above pass even with the split removed, because they
// exercise the geometry rather than the send path. This one reads what was
// ordered: the artillery and the tanks must be sent to different places.
func TestBaseAssaultOrdersSiegeToADifferentPlace(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	units := []model.Unit{
		{ID: 1, Type: "2tnk", X: 20, Y: 20, Idle: true},
		{ID: 2, Type: "2tnk", X: 21, Y: 20, Idle: true},
		{ID: 3, Type: "arty", X: 20, Y: 21, Idle: true},
		{ID: 4, Type: "e1", X: 21, Y: 21, Idle: true},
	}
	mem := map[string]any{
		"squads": map[string]*Squad{
			"ground-attack": {Name: "ground-attack", Domain: "ground", Role: "attack",
				UnitIDs: []int{1, 2, 3, 4}, TargetSize: 4},
		},
		"enemyBases": map[string]EnemyBaseIntel{
			"red": {Owner: "red", X: 100, Y: 100, Tick: 1, FromBuildings: true},
		},
	}
	env := RuleEnv{
		State:  model.GameState{Tick: 5000, MapWidth: 128, MapHeight: 128, Units: units},
		Memory: mem,
	}
	if err := SquadAttackKnownBase("ground-attack", 1.0)(env, conn); err != nil {
		t.Fatalf("assault: %v", err)
	}

	sent := memoryMap[int, attackMoveEntry](mem, "attackMoveSent")
	arty, okA := sent[3]
	tank, okT := sent[1]
	if !okA || !okT {
		t.Fatalf("missing orders: arty=%v tank=%v", okA, okT)
	}
	if arty.X == tank.X && arty.Y == tank.Y {
		t.Fatal("artillery was sent to the same point as the tanks: the range advantage is unused")
	}
	// The artillery must stop short, outside any base defence's reach.
	dx, dy := float64(arty.X-100), float64(arty.Y-100)
	gap := math.Hypot(dx, dy)
	if gap <= float64(maxBaseDefenceRange) {
		t.Errorf("artillery holds %.1f cells from the base, inside the %d-cell defence reach", gap, maxBaseDefenceRange)
	}
	if gap > float64(siegeRangeCells["arty"]) {
		t.Errorf("artillery holds %.1f cells out, beyond its own 20-cell gun", gap)
	}
}

// Focus fire concentrates on what the squad can already shoot; it does not
// chase.
//
// bestTargetForSquad scores from the squad position but returns anything,
// including targets behind it, and sendAttack on a distant actor is a move
// order in disguise. squad-focus-fire is category combat, so it acted alongside
// the exclusive assault winner: game 173 issued 522 attack-moves at the enemy
// base and 88 focus-fire orders elsewhere, and the squad sawtoothed roughly 23
// cells back and forth while its membership drained 17 to 12.
func TestFocusFireOnlyEngagesWhatIsInReach(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	units := []model.Unit{
		{ID: 1, Type: "2tnk", X: 50, Y: 50},
		{ID: 2, Type: "2tnk", X: 51, Y: 50},
	}
	run := func(enemyX, enemyY int) int {
		mem := map[string]any{"squads": map[string]*Squad{
			"ground-attack": {Name: "ground-attack", Domain: "ground", Role: "attack",
				UnitIDs: []int{1, 2}, TargetSize: 2},
		}}
		env := RuleEnv{
			State: model.GameState{
				Tick: 4000, MapWidth: 128, MapHeight: 128, Units: units,
				Enemies: []model.Enemy{{ID: 90, Type: "e1", X: enemyX, Y: enemyY, HP: 100, MaxHP: 100}},
			},
			Memory: mem,
		}
		if err := SquadFocusFire("ground-attack")(env, conn); err != nil {
			t.Fatalf("focus fire: %v", err)
		}
		return len(memoryMap[int, attackOrderEntry](mem, "attackOrderSent"))
	}

	// In contact: concentrating fire is the whole point of the rule.
	if run(54, 52) == 0 {
		t.Error("a target four cells away was not engaged")
	}
	// Most of a map away: that is a chase, and the assault rules own movement.
	if n := run(120, 120); n != 0 {
		t.Errorf("chased a distant target: %d orders issued", n)
	}
}
