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

func TestFormSquadNotEnoughUnits(t *testing.T) {
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
	err := action(env, nil)
	if err != nil {
		t.Fatalf("FormSquad action returned error: %v", err)
	}

	squads := getSquads(memory)
	if _, ok := squads["test-squad"]; ok {
		t.Error("expected no squad to be formed when insufficient units")
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

func TestSwapClearsSquads(t *testing.T) {
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

	if _, ok := engine.Memory["squads"]; ok {
		t.Error("expected squads to be cleared after Swap")
	}
}
