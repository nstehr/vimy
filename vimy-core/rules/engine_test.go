package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func TestDefaultRulesCompile(t *testing.T) {
	engine, err := NewEngine(DefaultRules())
	if err != nil {
		t.Fatalf("NewEngine(DefaultRules()) failed: %v", err)
	}
	if len(engine.rules) != 11 {
		t.Errorf("expected 11 rules, got %d", len(engine.rules))
	}
	// Verify priority ordering (descending).
	for i := 1; i < len(engine.rules); i++ {
		if engine.rules[i].Priority > engine.rules[i-1].Priority {
			t.Errorf("rules not sorted by priority: %s (%d) > %s (%d)",
				engine.rules[i].Name, engine.rules[i].Priority,
				engine.rules[i-1].Name, engine.rules[i-1].Priority)
		}
	}
}

func TestContainsType(t *testing.T) {
	buildings := []model.Building{
		{Type: "powr"},
		{Type: "Tent"},
		{Type: "proc"},
	}

	if !containsType(buildings, "tent") {
		t.Error("containsType should match case-insensitively: tent vs Tent")
	}
	if !containsType(buildings, "POWR") {
		t.Error("containsType should match case-insensitively: POWR vs powr")
	}
	if containsType(buildings, "barr") {
		t.Error("containsType should return false for missing type")
	}

	units := []model.Unit{
		{Type: "e1"},
		{Type: "E1"},
		{Type: "harv"},
	}

	if countType(units, "e1") != 2 {
		t.Errorf("countType(e1) = %d, want 2", countType(units, "e1"))
	}
	if countType(units, "mcv") != 0 {
		t.Errorf("countType(mcv) = %d, want 0", countType(units, "mcv"))
	}
}

func TestHasRole(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{Type: "powr"},
				{Type: "barr"},
			},
			Units: []model.Unit{
				{Type: "e1"},
			},
		},
	}

	if !env.HasRole("barracks") {
		t.Error("HasRole(barracks) should be true with barr building")
	}
	if !env.HasRole("power_plant") {
		t.Error("HasRole(power_plant) should be true with powr building")
	}
	if env.HasRole("war_factory") {
		t.Error("HasRole(war_factory) should be false without weap building")
	}
	if env.HasRole("nonexistent") {
		t.Error("HasRole(nonexistent) should be false for unknown role")
	}

	// Test with Allied barracks (tent).
	envAllied := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{Type: "tent"},
			},
		},
	}
	if !envAllied.HasRole("barracks") {
		t.Error("HasRole(barracks) should be true with tent building")
	}
}

func TestRoleCount(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{Type: "powr"},
				{Type: "powr"},
				{Type: "barr"},
			},
		},
	}

	if got := env.RoleCount("power_plant"); got != 2 {
		t.Errorf("RoleCount(power_plant) = %d, want 2", got)
	}
	if got := env.RoleCount("barracks"); got != 1 {
		t.Errorf("RoleCount(barracks) = %d, want 1", got)
	}
}

func TestCanBuildRole(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr", "barr", "proc"},
				},
			},
		},
	}

	if !env.CanBuildRole("barracks") {
		t.Error("CanBuildRole(barracks) should be true with barr in buildable")
	}
	if !env.CanBuildRole("power_plant") {
		t.Error("CanBuildRole(power_plant) should be true with powr in buildable")
	}
	if env.CanBuildRole("war_factory") {
		t.Error("CanBuildRole(war_factory) should be false without weap in buildable")
	}

	// Allied buildable list.
	envAllied := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr", "tent"},
				},
			},
		},
	}
	if !envAllied.CanBuildRole("barracks") {
		t.Error("CanBuildRole(barracks) should be true with tent in buildable")
	}
}

func TestBuildableType(t *testing.T) {
	// Soviet buildable: barr is available but tent is not.
	envSoviet := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr", "barr", "proc"},
				},
			},
		},
	}

	if got := envSoviet.BuildableType("barracks"); got != "barr" {
		t.Errorf("BuildableType(barracks) = %q, want %q", got, "barr")
	}

	// Allied buildable: tent is available but barr is not.
	envAllied := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr", "tent", "proc"},
				},
			},
		},
	}

	if got := envAllied.BuildableType("barracks"); got != "tent" {
		t.Errorf("BuildableType(barracks) = %q, want %q", got, "tent")
	}

	// Neither available.
	envNone := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr"},
				},
			},
		},
	}

	if got := envNone.BuildableType("barracks"); got != "" {
		t.Errorf("BuildableType(barracks) = %q, want %q", got, "")
	}

	// Unknown role.
	if got := envSoviet.BuildableType("nonexistent"); got != "" {
		t.Errorf("BuildableType(nonexistent) = %q, want %q", got, "")
	}
}
