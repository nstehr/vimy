package rules

import (
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
func TestFormSquadTakesNoMoreThanTargetSize(t *testing.T) {
	memory := make(map[string]any)
	units := make([]model.Unit, 0, 6)
	for i := range 6 {
		units = append(units, model.Unit{ID: 20 + i, Type: "1tnk", Idle: true})
	}
	env := RuleEnv{State: model.GameState{Units: units}, Memory: memory}

	if err := FormSquad("test-squad", "ground", 3, "attack")(env, nil); err != nil {
		t.Fatalf("FormSquad action returned error: %v", err)
	}

	sq := getSquads(memory)["test-squad"]
	if len(sq.UnitIDs) != 3 {
		t.Errorf("members = %d, want 3 — the surplus belongs to the other squads", len(sq.UnitIDs))
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

func TestSquadReadyRatio(t *testing.T) {
	memory := map[string]any{
		"squads": map[string]*Squad{
			"alpha": {
				Name:       "alpha",
				Domain:     "ground",
				UnitIDs:    []int{1, 2, 3, 4},
				Role:       "attack",
				TargetSize: 5,
			},
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "1tnk", Idle: true},
				{ID: 2, Type: "1tnk", Idle: true},
				{ID: 3, Type: "1tnk", Idle: true},
				{ID: 4, Type: "1tnk", Idle: false},
			},
		},
		Memory: memory,
	}

	ratio := env.SquadReadyRatio("alpha")
	if ratio != 0.75 {
		t.Errorf("expected SquadReadyRatio = 0.75, got %f", ratio)
	}
	if env.SquadReadyRatio("missing") != 0 {
		t.Error("expected SquadReadyRatio for missing squad to be 0")
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

	action := FormSquad("test-squad", "ground", 4, "attack")
	err := action(env, nil)
	if err != nil {
		t.Fatalf("FormSquad reinforcement returned error: %v", err)
	}

	squads := getSquads(memory)
	sq := squads["test-squad"]
	if sq == nil {
		t.Fatal("expected test-squad to exist")
	}
	if len(sq.UnitIDs) != 4 {
		t.Errorf("expected 4 units after reinforcement, got %d", len(sq.UnitIDs))
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
