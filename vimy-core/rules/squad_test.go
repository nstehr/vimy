package rules

import (
	"math"
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func TestUpdateSquadsPrunesDead(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"attack": {
				Name:    "attack",
				Domain:  "ground",
				UnitIDs: []int{1, 2, 3, 4, 5},
				Role:    "attack",
			},
		},
	}
	env := RuleEnv{
		State: model.GameState{
			// Units 2 and 4 are alive; 1, 3, 5 are dead.
			Units: []model.Unit{
				{ID: 2, Type: "1tnk", Idle: true},
				{ID: 4, Type: "1tnk", Idle: true},
			},
		},
		Memory: memory,
	}

	updateSquads(env)

	squads := getSquads(memory)
	sq, ok := squads["attack"]
	if !ok {
		t.Fatal("expected attack squad to still exist")
	}
	if len(sq.UnitIDs) != 2 {
		t.Errorf("expected 2 surviving units, got %d", len(sq.UnitIDs))
	}
	for _, id := range sq.UnitIDs {
		if id != 2 && id != 4 {
			t.Errorf("unexpected surviving unit ID: %d", id)
		}
	}
}

func TestUpdateSquadsDissolveEmpty(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"doomed": {
				Name:    "doomed",
				Domain:  "ground",
				UnitIDs: []int{10, 11},
				Role:    "attack",
			},
			"alive": {
				Name:    "alive",
				Domain:  "ground",
				UnitIDs: []int{20},
				Role:    "defend",
			},
		},
	}
	env := RuleEnv{
		State: model.GameState{
			// Only unit 20 is alive. Squad "doomed" has no survivors.
			Units: []model.Unit{
				{ID: 20, Type: "1tnk", Idle: true},
			},
		},
		Memory: memory,
	}

	updateSquads(env)

	squads := getSquads(memory)
	if _, ok := squads["doomed"]; ok {
		t.Error("expected doomed squad to be dissolved")
	}
	if _, ok := squads["alive"]; !ok {
		t.Error("expected alive squad to persist")
	}
}

func TestUnassignedIdleGround(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"attack": {
				Name:    "attack",
				Domain:  "ground",
				UnitIDs: []int{1, 2},
				Role:    "attack",
			},
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "1tnk", Idle: true},
				{ID: 2, Type: "1tnk", Idle: true},
				{ID: 3, Type: "1tnk", Idle: true},
				{ID: 4, Type: "1tnk", Idle: true},
				{ID: 5, Type: "1tnk", Idle: false}, // not idle
			},
		},
		Memory: memory,
	}

	unassigned := env.UnassignedIdleGround()
	if len(unassigned) != 2 {
		t.Errorf("expected 2 unassigned idle ground units, got %d", len(unassigned))
	}
	for _, u := range unassigned {
		if u.ID == 1 || u.ID == 2 {
			t.Errorf("unit %d is assigned to attack squad, should not be in unassigned pool", u.ID)
		}
	}
}

// Dogs are bite-only (anti-infantry); offensive squads and emergency-defense
// AttackMove dispatch should skip them so they don't get sent to attack
// vehicles they can't damage. Dogs remain usable as designated scouts and as
// in-place base auto-defense (game handles auto-fire when infantry is in range).
func TestIdleGroundUnits_ExcludesDogs(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "1tnk", Idle: true},
				{ID: 2, Type: "dog", Idle: true}, // should be excluded
				{ID: 3, Type: "e1", Idle: true},
			},
		},
		Memory: map[string]any{},
	}
	idle := env.IdleGroundUnits()
	for _, u := range idle {
		if u.Type == "dog" {
			t.Errorf("expected attack dog to be excluded from IdleGroundUnits, got id=%d", u.ID)
		}
	}
	if len(idle) != 2 {
		t.Errorf("expected 2 ground units (tank + rifle), got %d", len(idle))
	}
}

func TestNearBaseGroundUnits_ExcludesDogs(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			MapWidth:  100,
			MapHeight: 100,
			Buildings: []model.Building{
				{ID: 100, Type: "fact", X: 50, Y: 50},
			},
			Units: []model.Unit{
				{ID: 1, Type: "1tnk", X: 50, Y: 50},
				{ID: 2, Type: "dog", X: 50, Y: 50}, // should be excluded despite proximity
			},
		},
		Memory: map[string]any{},
	}
	nearby := env.NearBaseGroundUnits()
	for _, u := range nearby {
		if u.Type == "dog" {
			t.Errorf("expected attack dog to be excluded from NearBaseGroundUnits, got id=%d", u.ID)
		}
	}
}

func TestFormSquadAction(t *testing.T) {
	memory := make(map[string]any)
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 10, Type: "1tnk", Idle: true},
				{ID: 11, Type: "2tnk", Idle: true},
				{ID: 12, Type: "3tnk", Idle: true},
			},
		},
		Memory: memory,
	}

	action := FormSquad("test-squad", "ground", 3, "attack")
	err := action(env, nil)
	if err != nil {
		t.Fatalf("FormSquad action returned error: %v", err)
	}

	squads := getSquads(memory)
	sq, ok := squads["test-squad"]
	if !ok {
		t.Fatal("expected test-squad to exist in memory")
	}
	if sq.Name != "test-squad" {
		t.Errorf("expected squad name 'test-squad', got %q", sq.Name)
	}
	if sq.Domain != "ground" {
		t.Errorf("expected domain 'ground', got %q", sq.Domain)
	}
	if sq.Role != "attack" {
		t.Errorf("expected role 'attack', got %q", sq.Role)
	}
	if len(sq.UnitIDs) != 3 {
		t.Errorf("expected 3 unit IDs, got %d", len(sq.UnitIDs))
	}
}

// A short pool forms a short squad, which reinforcement then tops up.
//
// The rule's condition decides whether it is worth forming at all; this used to
// refuse anything below the full size, which is why no attack squad ever formed.
func TestFormSquadFormsBelowTargetSize(t *testing.T) {
	memory := make(map[string]any)
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 10, Type: "1tnk", Idle: true},
			},
		},
		Memory: memory,
	}

	action := FormSquad("test-squad", "ground", 3, "attack")
	if err := action(env, nil); err != nil {
		t.Fatalf("FormSquad action returned error: %v", err)
	}

	sq, ok := getSquads(memory)["test-squad"]
	if !ok {
		t.Fatal("expected a squad to form from a single unit")
	}
	if len(sq.UnitIDs) != 1 {
		t.Errorf("members = %d, want 1", len(sq.UnitIDs))
	}
	// The target is what was asked for, so SquadNeedsReinforcement stays true
	// and the formation rule keeps topping it up.
	if sq.TargetSize != 3 {
		t.Errorf("TargetSize = %d, want 3", sq.TargetSize)
	}
	if !(RuleEnv{State: env.State, Memory: memory}).SquadNeedsReinforcement("test-squad") {
		t.Error("a short squad should still want reinforcement")
	}
}

func TestFormSquadNeedsAtLeastOneUnit(t *testing.T) {
	memory := make(map[string]any)
	env := RuleEnv{State: model.GameState{}, Memory: memory}

	action := FormSquad("test-squad", "ground", 3, "attack")
	if err := action(env, nil); err != nil {
		t.Fatalf("FormSquad action returned error: %v", err)
	}

	if _, ok := getSquads(memory)["test-squad"]; ok {
		t.Error("expected no squad to be formed from an empty pool")
	}
}

// Formation never takes more than the target, however deep the pool.
func TestFormSquadTakesTheForceNotAQuota(t *testing.T) {
	memory := make(map[string]any)
	units := make([]model.Unit, 0, 6)
	for i := range 6 {
		units = append(units, model.Unit{ID: 20 + i, Type: "1tnk", Idle: true})
	}
	env := RuleEnv{State: model.GameState{Units: units}, Memory: memory}

	// The doctrine asks for three. Six are uncommitted, so it gets six.
	//
	// This used to assert exactly three, on the reasoning that "the surplus
	// belongs to the other squads". Ordering is what protects them, not a cap:
	// form-defense-squad runs at defend-priority()+5, around 475, against the
	// attack's attack-priority()+5, around 265, and both sit in squad-form. The
	// garrison has already taken what it needs by the time this runs, so what
	// is left is genuinely uncommitted.
	//
	// The cap cost real games. ground_attack_group_size is a constant, 5 or 6
	// in every doctrine the strategist has written, while the army runs from 7
	// combat units to 27 at peak — so the squad shrank as a fraction of the
	// army it was drawn from. Game 150 formed a full 6, held formation,
	// reached 0.143 of the map diagonal, and arrived with TWO units against 16
	// rocket soldiers and 10 APCs.
	if err := FormSquad("test-squad", "ground", 3, "attack")(env, nil); err != nil {
		t.Fatalf("FormSquad action returned error: %v", err)
	}

	sq := getSquads(memory)["test-squad"]
	if len(sq.UnitIDs) != 6 {
		t.Errorf("members = %d, want 6 — the doctrine size is a floor, not a quota", len(sq.UnitIDs))
	}
	if sq.TargetSize != 6 {
		t.Errorf("target = %d, want 6 — it tracks the committable force", sq.TargetSize)
	}
}

func TestSquadEnvMethods(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"alpha": {
				Name:    "alpha",
				Domain:  "ground",
				UnitIDs: []int{1, 2, 3},
				Role:    "attack",
			},
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "1tnk", Idle: true},
				{ID: 2, Type: "1tnk", Idle: false},
				{ID: 3, Type: "1tnk", Idle: true},
			},
		},
		Memory: memory,
	}

	if !env.SquadExists("alpha") {
		t.Error("expected SquadExists('alpha') to be true")
	}
	if env.SquadExists("beta") {
		t.Error("expected SquadExists('beta') to be false")
	}
	if env.SquadSize("alpha") != 3 {
		t.Errorf("expected SquadSize('alpha') = 3, got %d", env.SquadSize("alpha"))
	}
	if env.SquadSize("beta") != 0 {
		t.Errorf("expected SquadSize('beta') = 0, got %d", env.SquadSize("beta"))
	}
	if env.SquadIdleCount("alpha") != 2 {
		t.Errorf("expected SquadIdleCount('alpha') = 2, got %d", env.SquadIdleCount("alpha"))
	}
}

func TestSquadAttackMoveAction(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"attack": {
				Name:    "attack",
				Domain:  "ground",
				UnitIDs: []int{1, 2, 3},
				Role:    "attack",
			},
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "1tnk", Idle: true},
				{ID: 2, Type: "1tnk", Idle: true},
				{ID: 3, Type: "1tnk", Idle: false}, // not idle, won't be sent
			},
			Enemies: []model.Enemy{
				{ID: 99, X: 100, Y: 200},
			},
		},
		Memory: memory,
	}

	// Test that squadIdleActorIDs returns only idle members.
	ids := squadIdleActorIDs(env, "attack")
	if len(ids) != 2 {
		t.Errorf("expected 2 idle actor IDs, got %d", len(ids))
	}
}

func TestSquadNeedsReinforcement(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"under": {
				Name:       "under",
				Domain:     "ground",
				UnitIDs:    []int{1, 2},
				Role:       "attack",
				TargetSize: 5,
			},
			"full": {
				Name:       "full",
				Domain:     "ground",
				UnitIDs:    []int{10, 11, 12},
				Role:       "attack",
				TargetSize: 3,
			},
		},
	}
	env := RuleEnv{Memory: memory}

	if !env.SquadNeedsReinforcement("under") {
		t.Error("expected under-strength squad to need reinforcement")
	}
	if env.SquadNeedsReinforcement("full") {
		t.Error("expected full-strength squad to not need reinforcement")
	}
	if env.SquadNeedsReinforcement("missing") {
		t.Error("expected missing squad to not need reinforcement")
	}
}

// Readiness is how much of the INTENDED force is present, not how many members
// happen to have no current order.
//
// It counted Idle, and Idle clears the moment the sidecar issues an order, so a
// squad marching on its target read as zero ready while a lone stranded unit
// read as 1.0 — too strict for a real assault and too lax for a remnant. Game
// 158's squad decayed 15 to 6 to 2 to 1 and kept walking into the enemy base,
// because one unit of one is full marks.
func TestSquadReadyRatio(t *testing.T) {
	squad := func(ids []int, target int) map[string]any {
		return map[string]any{"squads": map[string]*Squad{
			"alpha": {Name: "alpha", Domain: "ground", Role: "attack", UnitIDs: ids, TargetSize: target},
		}}
	}
	units := func(ids []int, idle bool) []model.Unit {
		var us []model.Unit
		for _, id := range ids {
			us = append(us, model.Unit{ID: id, Type: "1tnk", Idle: idle})
		}
		return us
	}

	cases := []struct {
		name   string
		ids    []int
		alive  []int
		idle   bool
		target int
		want   float64
	}{
		{"four of five present", []int{1, 2, 3, 4}, []int{1, 2, 3, 4}, true, 5, 0.8},
		// The whole point: orders must not lower readiness.
		{"a marching squad is ready", []int{1, 2, 3, 4, 5}, []int{1, 2, 3, 4, 5}, false, 5, 1.0},
		// Game 158: a remnant must not read as a ready squad.
		{"a remnant of one is not ready", []int{1}, []int{1}, true, 6, 1.0 / 6.0},
		// Dead members are not present, however large the roster says it is.
		{"the dead do not count", []int{1, 2, 3, 4, 5, 6}, []int{1, 2}, true, 6, 2.0 / 6.0},
		{"over strength caps at one", []int{1, 2, 3, 4}, []int{1, 2, 3, 4}, true, 2, 1.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := RuleEnv{
				State:  model.GameState{Units: units(c.alive, c.idle)},
				Memory: squad(c.ids, c.target),
			}
			if got := env.SquadReadyRatio("alpha"); math.Abs(got-c.want) > 1e-9 {
				t.Errorf("ready ratio = %v, want %v", got, c.want)
			}
		})
	}

	env := RuleEnv{State: model.GameState{}, Memory: squad([]int{1}, 5)}
	if env.SquadReadyRatio("missing") != 0 {
		t.Error("expected SquadReadyRatio for missing squad to be 0")
	}
}

// A squad limping to the depot is not ready to assault, so retreating members
// leave the numerator while TargetSize stands.
func TestSquadReadyRatioExcludesRetreating(t *testing.T) {
	mem := map[string]any{"squads": map[string]*Squad{
		"alpha": {Name: "alpha", Domain: "ground", Role: "attack", UnitIDs: []int{1, 2, 3, 4}, TargetSize: 4},
	}}
	env := RuleEnv{
		State: model.GameState{Units: []model.Unit{
			{ID: 1, Type: "1tnk"}, {ID: 2, Type: "1tnk"},
			{ID: 3, Type: "1tnk"}, {ID: 4, Type: "1tnk"},
		}},
		Memory: mem,
	}
	if got := env.SquadReadyRatio("alpha"); got != 1.0 {
		t.Fatalf("ready ratio = %v, want 1 with everyone present", got)
	}
	mem["retreatingUnits"] = map[int]int{3: 100, 4: 100}
	if got := env.SquadReadyRatio("alpha"); got != 0.5 {
		t.Errorf("ready ratio = %v, want 0.5 with half the squad retreating", got)
	}
}

func TestFormSquadReinforcement(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"test-squad": {
				Name:       "test-squad",
				Domain:     "ground",
				UnitIDs:    []int{1, 2},
				Role:       "attack",
				TargetSize: 4,
			},
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "1tnk", Idle: true},
				{ID: 2, Type: "1tnk", Idle: true},
				{ID: 3, Type: "1tnk", Idle: true},
				{ID: 4, Type: "1tnk", Idle: true},
			},
		},
		Memory: memory,
	}

	conn, cleanup := testConn(t)
	defer cleanup()

	action := FormSquad("test-squad", "ground", 4, "attack")
	err := action(env, conn)
	if err != nil {
		t.Fatalf("FormSquad reinforcement returned error: %v", err)
	}

	squads := getSquads(memory)
	sq := squads["test-squad"]
	if sq == nil {
		t.Fatal("expected test-squad to exist")
	}
	// Reinforcements are dispatched, not enlisted: they join the roster once
	// they reach the squad, so the total is what tops up to TargetSize.
	if got := len(sq.UnitIDs) + len(sq.Joining); got != 4 {
		t.Errorf("expected 4 units dispatched or enlisted, got %d (%d members, %d joining)",
			got, len(sq.UnitIDs), len(sq.Joining))
	}
	// Original units should still be present.
	if sq.UnitIDs[0] != 1 || sq.UnitIDs[1] != 2 {
		t.Error("expected original unit IDs to be preserved")
	}
}

func TestHuntStateCleanupOnDissolve(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"doomed": {
				Name:       "doomed",
				Domain:     "ground",
				UnitIDs:    []int{10, 11},
				Role:       "attack",
				TargetSize: 5,
			},
		},
		"huntBase:doomed": &huntBaseState{BaseX: 100, BaseY: 200, Step: 3},
	}
	env := RuleEnv{
		State: model.GameState{
			// No surviving units — squad will dissolve.
			Units: []model.Unit{},
		},
		Memory: memory,
	}

	updateSquads(env)

	if _, ok := memory["huntBase:doomed"]; ok {
		t.Error("expected huntBase:doomed to be cleaned up on dissolution")
	}
}

func TestSquadIdleActorIDs_SkipsRetreating(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"attack": {
				Name:    "attack",
				Domain:  "ground",
				UnitIDs: []int{1, 2, 3},
				Role:    "attack",
			},
		},
		"retreatingUnits": map[int]int{2: 0},
	}
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "1tnk", Idle: true},
				{ID: 2, Type: "1tnk", Idle: true}, // retreating, should be skipped
				{ID: 3, Type: "1tnk", Idle: true},
			},
		},
		Memory: memory,
	}

	ids := squadIdleActorIDs(env, "attack")
	if len(ids) != 2 {
		t.Errorf("expected 2 idle non-retreating actor IDs, got %d", len(ids))
	}
	for _, id := range ids {
		if id == 2 {
			t.Error("retreating unit 2 should not be in squadIdleActorIDs")
		}
	}
}

// The seed rules form no squads, so a swap to them orphans anything standing.
//
// Asserts the roster is empty rather than that the "squads" key is absent: a
// swap now keeps the squads the new rules still form, so the key survives and
// only the orphans are removed.
func TestSwapClearsSquadsTheSeedRulesCannotForm(t *testing.T) {
	engine, err := NewEngine(DefaultRules())
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Simulate squads in memory.
	engine.Memory["squads"] = map[string]*Squad{
		"attack": {Name: "attack", UnitIDs: []int{1, 2, 3}},
	}

	err = engine.Swap(DefaultRules())
	if err != nil {
		t.Fatalf("Swap failed: %v", err)
	}

	if n := len(getSquads(engine.Memory)); n != 0 {
		t.Errorf("expected no squads after a swap to rules that form none, got %d", n)
	}
}

// A squad in combat is not idle, and combat is the only situation
// squad-disengage runs in: the rule requires the squad away from base and
// outnumbered. Gating the action on idleness meant that at the moment it most
// needed to withdraw it had nobody to order. Games 88, 90 and 91:
// squad-disengage-ground-attack matched 155 times and ordered something 10.
func TestSquadDisengageOrdersUnitsThatAreFighting(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"ground-attack": {Name: "ground-attack", UnitIDs: []int{1, 2, 3}},
		},
	}
	env := RuleEnv{
		Memory: memory,
		State: model.GameState{
			Tick: 1000,
			// None idle: every member is engaged.
			Units: []model.Unit{
				{ID: 1, X: 900, Y: 900, Idle: false},
				{ID: 2, X: 910, Y: 900, Idle: false},
				{ID: 3, X: 900, Y: 910, Idle: false},
			},
			Buildings: []model.Building{{ID: 9, Type: "fact", X: 100, Y: 100}},
		},
	}
	conn, cleanup := testConn(t)
	defer cleanup()

	before := conn.Sent()
	if err := SquadDisengage("ground-attack")(env, conn); err != nil {
		t.Fatal(err)
	}
	if conn.Sent() == before {
		t.Fatal("a squad locked in combat was given no retreat order")
	}

	// And it must not re-order every tick: that cancels the in-flight path and
	// leaves the squad standing still under fire.
	mid := conn.Sent()
	if err := SquadDisengage("ground-attack")(env, conn); err != nil {
		t.Fatal(err)
	}
	if conn.Sent() != mid {
		t.Error("re-issued the same retreat order immediately, cancelling the path")
	}
}

// squad-reengage sends any idle squad member at the enemy the moment it stops
// moving — 32 acts in game 95 against 8 deliberate attacks. Without a hold, a
// squad ordered to withdraw walks home, goes idle on arrival and is sent
// straight back, so the retreat is undone by the busiest rule in the game.
func TestDisengagedUnitsAreNotImmediatelySentBack(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"ground-attack": {Name: "ground-attack", UnitIDs: []int{1, 2}},
		},
	}
	env := func(tick int, idle bool) RuleEnv {
		return RuleEnv{
			Memory: memory,
			State: model.GameState{
				Tick: tick,
				Units: []model.Unit{
					{ID: 1, X: 900, Y: 900, Idle: idle},
					{ID: 2, X: 910, Y: 900, Idle: idle},
				},
				Buildings: []model.Building{{ID: 9, Type: "fact", X: 100, Y: 100}},
			},
		}
	}
	conn, cleanup := testConn(t)
	defer cleanup()

	// Withdraw while fighting.
	if err := SquadDisengage("ground-attack")(env(1000, false), conn); err != nil {
		t.Fatal(err)
	}
	// They arrive home and go idle: the attacking rules must not see them yet.
	if ids := squadIdleActorIDs(env(1100, true), "ground-attack"); len(ids) != 0 {
		t.Errorf("a squad that just withdrew was offered to the attacking rules: %v", ids)
	}
	// After the hold expires they are available again.
	if ids := squadIdleActorIDs(env(1000+disengageHoldTicks+1, true), "ground-attack"); len(ids) != 2 {
		t.Errorf("units stayed benched after the hold expired: got %d, want 2", len(ids))
	}
}

// A joiner becomes a member when it arrives, not when it is dispatched.
//
// Counting it on dispatch is what let a 20-member squad sit at spread 60 with
// ZERO members inside the clump radius: the units enlisted from base were sixty
// cells behind by definition. That failed the clump gate, which cleared
// Attacking, which reset the approach to step zero, which sent the squad back
// out to the detour waypoint - a loop swinging across 45% of the map diagonal.
func TestJoinersCountOnArrivalNotOnDispatch(t *testing.T) {
	mem := map[string]any{"squads": map[string]*Squad{
		"ground-attack": {
			Name: "ground-attack", Domain: "ground", Role: "attack",
			UnitIDs: []int{1, 2}, Joining: []int{3, 4}, TargetSize: 4,
		},
	}}
	// 1 and 2 hold the line at (100,100). 3 is still walking; 4 has arrived.
	env := RuleEnv{
		State: model.GameState{Tick: 9000, Units: []model.Unit{
			{ID: 1, Type: "2tnk", X: 100, Y: 100},
			{ID: 2, Type: "2tnk", X: 102, Y: 100},
			{ID: 3, Type: "2tnk", X: 20, Y: 20},
			{ID: 4, Type: "2tnk", X: 104, Y: 101},
		}},
		Memory: mem,
	}
	updateSquads(env)
	sq := getSquads(mem)["ground-attack"]

	if !containsID(sq.UnitIDs, 4) {
		t.Error("a joiner standing with the squad was not enlisted")
	}
	if containsID(sq.UnitIDs, 3) {
		t.Error("a joiner 80 cells away was enlisted: it will read as a scattered squad")
	}
	if !containsID(sq.Joining, 3) {
		t.Error("the distant joiner was dropped instead of kept walking")
	}

	// Cohesion must judge the squad that exists, not the one in transit.
	env2 := RuleEnv{State: env.State, Memory: mem}
	members, _, _ := env2.SquadClump("ground-attack", 8)
	if members != 3 {
		t.Errorf("clump saw %d members, want 3 - the walker must not count", members)
	}

	// A dead joiner is dropped rather than walked forever.
	mem2 := map[string]any{"squads": map[string]*Squad{
		"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{1}, Joining: []int{9}, TargetSize: 2},
	}}
	dead := RuleEnv{State: model.GameState{Tick: 9100, Units: []model.Unit{{ID: 1, Type: "2tnk", X: 5, Y: 5}}}, Memory: mem2}
	updateSquads(dead)
	if got := getSquads(mem2)["ground-attack"]; len(got.Joining) != 0 {
		t.Errorf("a dead joiner is still joining: %v", got.Joining)
	}
}

func containsID(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// A unit dispatched to a squad must not be dispatched again, and must not be
// enlisted twice if it is.
//
// squadUnitIDSet, which is what UnassignedIdleGround subtracts to find who is
// free, walks only UnitIDs. A joiner sits in Joining -- by design, so cohesion
// judges the squad that exists rather than the one in transit -- and so still
// reads as unassigned, gets dispatched a second time, and is enlisted once per
// entry when it arrives.
//
// The roster then holds the same id twice, and the two ways of counting a squad
// disagree: squadActorIDs walks the slice and counts it twice, SquadClump keys a
// map off the roster and counts it once. That is visible in the telemetry as
// idle > members, which is impossible by construction -- every rally in session
// 20260925-183637 reported it, 12 members against 13 idle. It also inflates
// len(sq.UnitIDs), which is what `need` subtracts when topping a squad up, so
// the squad under-reinforces for the rest of the game.
func TestJoiningUnitIsNotEnlistedTwice(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"attack": {
				Name: "attack", Domain: "ground", Role: "attack", TargetSize: 4,
				UnitIDs: []int{1, 2},
				// Dispatched twice, because the first dispatch left it looking
				// unassigned.
				Joining: []int{3, 3},
			},
		},
	}
	env := RuleEnv{
		State: model.GameState{Units: []model.Unit{
			{ID: 1, Type: "1tnk", X: 10, Y: 10},
			{ID: 2, Type: "1tnk", X: 11, Y: 10},
			// Arrived: within joinArrivedCells of the squad centroid.
			{ID: 3, Type: "1tnk", X: 12, Y: 10},
		}},
		Memory: memory,
	}

	updateSquads(env)

	sq := getSquads(memory)["attack"]
	seen := map[int]int{}
	for _, id := range sq.UnitIDs {
		seen[id]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("unit %d is on the roster %d times", id, n)
		}
	}

	// The two ways of counting the squad have to agree. idle counts roster
	// entries, members counts roster units present on the field.
	idle := len(squadAssaultActorIDs(env, "attack"))
	members, _, _ := env.SquadClump("attack", squadRallyRadius)
	if idle > members {
		t.Errorf("idle %d exceeds members %d: a squad cannot command more units than it has",
			idle, members)
	}
}

// A unit already in Joining must not read as unassigned, or it is dispatched
// again on the next evaluation -- which is how it gets into Joining twice.
func TestJoiningUnitIsNotUnassigned(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"attack": {
				Name: "attack", Domain: "ground", Role: "attack", TargetSize: 4,
				UnitIDs: []int{1},
				Joining: []int{3},
			},
		},
	}
	env := RuleEnv{
		State: model.GameState{Units: []model.Unit{
			{ID: 1, Type: "1tnk", X: 10, Y: 10, Idle: true},
			{ID: 3, Type: "1tnk", X: 90, Y: 90, Idle: true},
		}},
		Memory: memory,
	}

	for _, u := range env.UnassignedIdleGround() {
		if u.ID == 3 {
			t.Error("unit 3 is already walking to the attack squad and must not read as unassigned")
		}
	}
}
