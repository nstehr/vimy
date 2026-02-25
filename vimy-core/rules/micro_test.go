package rules

import (
	"testing"

	"github.com/expr-lang/expr"
	"github.com/nstehr/vimy/vimy-core/model"
)

func TestDamagedSquadUnits(t *testing.T) {
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
				{ID: 1, Type: "1tnk", Idle: true, HP: 30, MaxHP: 100},  // 30% — below 0.5
				{ID: 2, Type: "1tnk", Idle: true, HP: 80, MaxHP: 100},  // 80% — above 0.5
				{ID: 3, Type: "1tnk", Idle: false, HP: 20, MaxHP: 100}, // damaged but not idle
				{ID: 4, Type: "1tnk", Idle: true, HP: 10, MaxHP: 100},  // not in squad
				{ID: 5, Type: "1tnk", Idle: true, HP: 0, MaxHP: 0},     // MaxHP == 0, excluded
			},
		},
		Memory: memory,
	}

	damaged := env.DamagedSquadUnits(0.50)

	if len(damaged) != 1 {
		t.Fatalf("expected 1 damaged squad unit, got %d", len(damaged))
	}
	if damaged[0].ID != 1 {
		t.Errorf("expected unit ID 1, got %d", damaged[0].ID)
	}
}

func TestServiceDepotOrCentroid(t *testing.T) {
	t.Run("with service depot", func(t *testing.T) {
		env := RuleEnv{
			State: model.GameState{
				Buildings: []model.Building{
					{ID: 1, Type: "fact", X: 100, Y: 100},
					{ID: 2, Type: "fix", X: 200, Y: 200},
				},
			},
			Memory: make(map[string]any),
		}
		x, y := env.ServiceDepotOrCentroid()
		if x != 200 || y != 200 {
			t.Errorf("expected service depot (200,200), got (%d,%d)", x, y)
		}
	})

	t.Run("without service depot", func(t *testing.T) {
		env := RuleEnv{
			State: model.GameState{
				Buildings: []model.Building{
					{ID: 1, Type: "fact", X: 100, Y: 100},
					{ID: 2, Type: "powr", X: 200, Y: 200},
				},
			},
			Memory: make(map[string]any),
		}
		x, y := env.ServiceDepotOrCentroid()
		// Centroid: (100+200)/2=150, (100+200)/2=150
		if x != 150 || y != 150 {
			t.Errorf("expected centroid (150,150), got (%d,%d)", x, y)
		}
	})

	t.Run("no buildings", func(t *testing.T) {
		env := RuleEnv{
			State:  model.GameState{},
			Memory: make(map[string]any),
		}
		x, y := env.ServiceDepotOrCentroid()
		if x != 0 || y != 0 {
			t.Errorf("expected (0,0), got (%d,%d)", x, y)
		}
	})
}

func TestWeakestVisibleEnemy(t *testing.T) {
	t.Run("picks lowest ratio", func(t *testing.T) {
		env := RuleEnv{
			State: model.GameState{
				Buildings: []model.Building{{ID: 1, Type: "fact", X: 0, Y: 0}},
				Enemies: []model.Enemy{
					{ID: 10, X: 50, Y: 50, HP: 80, MaxHP: 100},  // 80%
					{ID: 11, X: 60, Y: 60, HP: 30, MaxHP: 100},  // 30% — weakest
					{ID: 12, X: 70, Y: 70, HP: 100, MaxHP: 100}, // 100%
				},
			},
			Memory: make(map[string]any),
		}
		weakest := env.WeakestVisibleEnemy()
		if weakest == nil {
			t.Fatal("expected non-nil enemy")
		}
		if weakest.ID != 11 {
			t.Errorf("expected enemy ID 11, got %d", weakest.ID)
		}
	})

	t.Run("breaks ties by proximity", func(t *testing.T) {
		env := RuleEnv{
			State: model.GameState{
				Buildings: []model.Building{{ID: 1, Type: "fact", X: 0, Y: 0}},
				Enemies: []model.Enemy{
					{ID: 10, X: 100, Y: 100, HP: 50, MaxHP: 100}, // 50%, farther
					{ID: 11, X: 10, Y: 10, HP: 50, MaxHP: 100},   // 50%, closer
				},
			},
			Memory: make(map[string]any),
		}
		weakest := env.WeakestVisibleEnemy()
		if weakest == nil {
			t.Fatal("expected non-nil enemy")
		}
		if weakest.ID != 11 {
			t.Errorf("expected closer enemy ID 11, got %d", weakest.ID)
		}
	})

	t.Run("nil when no enemies", func(t *testing.T) {
		env := RuleEnv{
			State:  model.GameState{},
			Memory: make(map[string]any),
		}
		if env.WeakestVisibleEnemy() != nil {
			t.Error("expected nil when no enemies")
		}
	})

	t.Run("skips MaxHP zero", func(t *testing.T) {
		env := RuleEnv{
			State: model.GameState{
				Enemies: []model.Enemy{
					{ID: 10, X: 50, Y: 50, HP: 0, MaxHP: 0},
				},
			},
			Memory: make(map[string]any),
		}
		if env.WeakestVisibleEnemy() != nil {
			t.Error("expected nil when all enemies have MaxHP == 0")
		}
	})
}

func TestHarvestersInDanger(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			MapWidth:  1000,
			MapHeight: 1000,
			Units: []model.Unit{
				{ID: 1, Type: "harv", Idle: true, X: 100, Y: 100},   // near enemy
				{ID: 2, Type: "harv", Idle: false, X: 110, Y: 110},  // near enemy, not idle — still included
				{ID: 3, Type: "harv", Idle: true, X: 900, Y: 900},   // far from enemy
				{ID: 4, Type: "1tnk", Idle: true, X: 100, Y: 100},   // not a harvester
			},
			Enemies: []model.Enemy{
				{ID: 99, X: 120, Y: 120, HP: 100, MaxHP: 100},
			},
		},
		Memory: make(map[string]any),
	}

	// dangerPct = 0.10 → threshold ≈ 141.4 (10% of ~1414 diagonal)
	harvesters := env.HarvestersInDanger(0.10)

	if len(harvesters) != 2 {
		t.Fatalf("expected 2 harvesters in danger, got %d", len(harvesters))
	}
	ids := map[int]bool{}
	for _, u := range harvesters {
		ids[u.ID] = true
	}
	if !ids[1] || !ids[2] {
		t.Errorf("expected harvesters 1 and 2 in danger, got IDs %v", ids)
	}
	if ids[3] {
		t.Error("harvester 3 is far and should not be in danger")
	}
}

func TestCompileDoctrineMicroRules(t *testing.T) {
	d := DefaultDoctrine() // Aggression=0.5, EconomyPriority=0.5
	rules := CompileDoctrine(d)

	// Verify all rules compile with expr.
	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	found := map[string]bool{}
	for _, r := range rules {
		found[r.Name] = true
	}

	// All three micro rules should be present for default doctrine.
	if !found["retreat-damaged-units"] {
		t.Error("expected retreat-damaged-units rule")
	}
	if !found["squad-focus-fire"] {
		t.Error("expected squad-focus-fire rule (Aggression=0.5 > 0.2)")
	}
	if !found["flee-harvesters"] {
		t.Error("expected flee-harvesters rule (EconomyPriority=0.5 > 0.1)")
	}

	// All micro rules should use category "micro" and be non-exclusive.
	for _, r := range rules {
		if r.Name == "retreat-damaged-units" || r.Name == "squad-focus-fire" || r.Name == "flee-harvesters" {
			if r.Category != "micro" {
				t.Errorf("rule %q should have category 'micro', got %q", r.Name, r.Category)
			}
			if r.Exclusive {
				t.Errorf("rule %q should not be exclusive", r.Name)
			}
		}
	}
}

func TestCompileDoctrineMicroRulesGating(t *testing.T) {
	// Low aggression, low economy → focus-fire and flee gated out.
	d := Doctrine{
		Name:                  "Passive",
		EconomyPriority:       0.05, // below DoctrineEnabled
		Aggression:            0.1,  // below DoctrineModerate
		InfantryWeight:        0.3,
		VehicleWeight:         0.3,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
	}
	rules := CompileDoctrine(d)

	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	found := map[string]bool{}
	for _, r := range rules {
		found[r.Name] = true
	}

	// Retreat is always present.
	if !found["retreat-damaged-units"] {
		t.Error("expected retreat-damaged-units even with low aggression")
	}
	// Focus fire gated by Aggression > DoctrineModerate.
	if found["squad-focus-fire"] {
		t.Error("unexpected squad-focus-fire with Aggression=0.1")
	}
	// Flee harvesters gated by EconomyPriority > DoctrineEnabled.
	if found["flee-harvesters"] {
		t.Error("unexpected flee-harvesters with EconomyPriority=0.05")
	}
}
