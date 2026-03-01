package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func TestDamagedCombatUnits(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", HP: 30, MaxHP: 100, Idle: false}, // damaged heavy tank, fighting
				{ID: 2, Type: "3tnk", HP: 80, MaxHP: 100, Idle: true},  // healthy heavy tank
				{ID: 3, Type: "e1", HP: 10, MaxHP: 50, Idle: true},     // damaged infantry
				{ID: 4, Type: "harv", HP: 10, MaxHP: 100, Idle: true},  // damaged harvester — excluded
				{ID: 5, Type: "mcv", HP: 10, MaxHP: 100, Idle: true},   // damaged MCV — excluded
				{ID: 6, Type: "jeep", HP: 10, MaxHP: 100, Idle: true},  // damaged ranger — excluded
				{ID: 7, Type: "e6", HP: 10, MaxHP: 50, Idle: true},     // damaged engineer — excluded
				{ID: 8, Type: "apc", HP: 10, MaxHP: 100, Idle: true},   // damaged APC — excluded
				{ID: 9, Type: "1tnk", HP: 20, MaxHP: 100, Idle: true},  // damaged light tank
			},
		},
		Memory: make(map[string]any),
	}

	got := env.DamagedCombatUnits(0.50)
	wantIDs := map[int]bool{1: true, 9: true} // infantry (ID=3) excluded — can't heal
	if len(got) != len(wantIDs) {
		t.Fatalf("DamagedCombatUnits: got %d units, want %d", len(got), len(wantIDs))
	}
	for _, u := range got {
		if !wantIDs[u.ID] {
			t.Errorf("unexpected unit ID %d (type %s) in result", u.ID, u.Type)
		}
	}
}

func TestDamagedCombatUnits_SkipsRetreating(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", HP: 20, MaxHP: 100, Idle: true},
				{ID: 2, Type: "1tnk", HP: 20, MaxHP: 100, Idle: true},
			},
		},
		Memory: map[string]any{
			"retreatingUnits": map[int]bool{1: true},
		},
	}

	got := env.DamagedCombatUnits(0.50)
	if len(got) != 1 || got[0].ID != 2 {
		t.Errorf("expected only unit 2, got %v", got)
	}
}

func TestOverextendedSquadMembers(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			MapWidth:  1000,
			MapHeight: 1000,
			Buildings: []model.Building{
				{ID: 100, Type: "fact", X: 100, Y: 100},
			},
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 100, Y: 100, Idle: true},  // near base
				{ID: 2, Type: "3tnk", X: 900, Y: 900, Idle: true},  // far from base
				{ID: 3, Type: "3tnk", X: 800, Y: 800, Idle: false}, // far but not idle
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {
					Name:       "ground-attack",
					Domain:     "ground",
					UnitIDs:    []int{1, 2, 3},
					TargetSize: 3,
				},
			},
		},
	}

	// 25% of diagonal (~353) — unit 2 at distance ~1131 should be overextended
	got := env.OverextendedSquadMembers("ground-attack", 0.25)
	if len(got) != 1 || got[0].ID != 2 {
		t.Errorf("expected unit 2 overextended, got %v", got)
	}
}

func TestOverextendedSquadMembers_NoSquad(t *testing.T) {
	env := RuleEnv{
		State:  model.GameState{MapWidth: 1000, MapHeight: 1000},
		Memory: make(map[string]any),
	}
	got := env.OverextendedSquadMembers("nonexistent", 0.25)
	if got != nil {
		t.Errorf("expected nil for nonexistent squad, got %v", got)
	}
}

func TestSquadThreatRatio(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			MapWidth:  1000,
			MapHeight: 1000,
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 500, Y: 500, HP: 100, MaxHP: 100},
				{ID: 2, Type: "3tnk", X: 510, Y: 500, HP: 100, MaxHP: 100},
			},
			Enemies: []model.Enemy{
				{ID: 10, X: 520, Y: 500, HP: 400, MaxHP: 400}, // nearby, big HP
				{ID: 11, X: 900, Y: 900, HP: 9999, MaxHP: 9999}, // far away
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {
					Name:       "ground-attack",
					Domain:     "ground",
					UnitIDs:    []int{1, 2},
					TargetSize: 2,
				},
			},
		},
	}

	// 10% of diagonal (~141). Enemy 10 is ~17 away (within radius).
	// Enemy 11 is ~566 away (outside). Squad HP=200, enemy HP near=400.
	// Ratio = 400/200 = 2.0
	ratio := env.SquadThreatRatio("ground-attack", 0.10)
	if ratio < 1.9 || ratio > 2.1 {
		t.Errorf("expected ratio ~2.0, got %.2f", ratio)
	}
}

func TestSquadThreatRatio_NoEnemies(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			MapWidth:  1000,
			MapHeight: 1000,
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 500, Y: 500, HP: 100, MaxHP: 100},
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {
					Name:       "ground-attack",
					Domain:     "ground",
					UnitIDs:    []int{1},
					TargetSize: 1,
				},
			},
		},
	}

	ratio := env.SquadThreatRatio("ground-attack", 0.10)
	if ratio != 0 {
		t.Errorf("expected 0 with no enemies, got %.2f", ratio)
	}
}

func TestBuildingCentroid(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{X: 100, Y: 200},
				{X: 300, Y: 400},
			},
		},
	}
	x, y := env.BuildingCentroid()
	if x != 200 || y != 300 {
		t.Errorf("expected (200, 300), got (%d, %d)", x, y)
	}
}

func TestBuildingCentroid_Empty(t *testing.T) {
	env := RuleEnv{State: model.GameState{}}
	x, y := env.BuildingCentroid()
	if x != 0 || y != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", x, y)
	}
}

func TestServiceDepotPos(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{Type: "fact", X: 100, Y: 100},
				{Type: "fix", X: 200, Y: 300},
			},
		},
	}
	x, y, ok := env.ServiceDepotPos()
	if !ok || x != 200 || y != 300 {
		t.Errorf("expected (200, 300, true), got (%d, %d, %v)", x, y, ok)
	}
}

func TestServiceDepotPos_NoDepot(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{Type: "fact", X: 100, Y: 100},
			},
		},
	}
	_, _, ok := env.ServiceDepotPos()
	if ok {
		t.Error("expected no service depot")
	}
}

func TestIsInfantry(t *testing.T) {
	tests := []struct {
		unitType string
		want     bool
	}{
		{"e1", true},
		{"e3", true},
		{"e6", true},
		{"e4", true},
		{"shok", true},
		{"e7", true},
		{"medi", true},
		{"3tnk", false},
		{"harv", false},
		{"heli", false},
	}
	for _, tt := range tests {
		u := model.Unit{Type: tt.unitType}
		if got := isInfantry(u); got != tt.want {
			t.Errorf("isInfantry(%q) = %v, want %v", tt.unitType, got, tt.want)
		}
	}
}

func TestBestBuildableVehicle_Preferences(t *testing.T) {
	// Both heavy_tank (mammoth) and light_tank are buildable,
	// but preferences ask for light_tank first.
	env := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{Type: "Vehicle", Buildable: []string{"1tnk", "4tnk"}},
			},
		},
		Memory: make(map[string]any),
		Preferences: UnitPreferences{
			Vehicle: []string{"light_tank", "medium_tank"},
		},
	}
	got := env.BestBuildableVehicle()
	if got != "1tnk" {
		t.Errorf("BestBuildableVehicle with preferences: got %q, want %q", got, "1tnk")
	}
}

func TestBestBuildableVehicle_EmptyPreferencesFallback(t *testing.T) {
	// No preferences set — should fall back to hardcoded priority (heavy_tank first).
	env := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{Type: "Vehicle", Buildable: []string{"1tnk", "4tnk"}},
			},
		},
		Memory: make(map[string]any),
	}
	got := env.BestBuildableVehicle()
	if got != "4tnk" {
		t.Errorf("BestBuildableVehicle without preferences: got %q, want %q (heavy_tank)", got, "4tnk")
	}
}

func TestBestBuildableSpecialist_Preferences(t *testing.T) {
	// Preferences ask for flamethrower first, even though tanya is higher priority.
	env := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{Type: "Infantry", Buildable: []string{"e4", "e7"}},
			},
		},
		Memory: make(map[string]any),
		Preferences: UnitPreferences{
			Infantry: []string{"flamethrower"},
		},
	}
	got := env.BestBuildableSpecialist()
	if got != "e4" {
		t.Errorf("BestBuildableSpecialist with preferences: got %q, want %q", got, "e4")
	}
}

func TestBestBuildableVehicle_UnknownPreferenceSkipped(t *testing.T) {
	// Unknown role name in preferences is silently skipped, falls back to hardcoded.
	env := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{Type: "Vehicle", Buildable: []string{"1tnk"}},
			},
		},
		Memory: make(map[string]any),
		Preferences: UnitPreferences{
			Vehicle: []string{"nonexistent_tank"},
		},
	}
	got := env.BestBuildableVehicle()
	if got != "1tnk" {
		t.Errorf("BestBuildableVehicle with unknown preference: got %q, want %q", got, "1tnk")
	}
}

func TestBestGroundTarget(t *testing.T) {
	base := model.Building{X: 0, Y: 0}

	t.Run("prefers tesla coil over naval yard at same distance", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "syrd", X: 10, Y: 0, HP: 200, MaxHP: 200},
				{ID: 2, Type: "tsla", X: 10, Y: 0, HP: 200, MaxHP: 200},
			},
		}}
		got := env.BestGroundTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected tsla (ID=2), got %+v", got)
		}
	})

	t.Run("prefers defense over production at same distance", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "weap", X: 10, Y: 0, HP: 200, MaxHP: 200},
				{ID: 2, Type: "gun", X: 10, Y: 0, HP: 100, MaxHP: 100},
			},
		}}
		got := env.BestGroundTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected gun (ID=2), got %+v", got)
		}
	})

	t.Run("nearby lower-value beats distant higher-value", func(t *testing.T) {
		// gun at dist=5: score = 8 * 1.0 / 5 = 1.6
		// tsla at dist=100: score = 10 * 1.0 / 100 = 0.1
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "gun", X: 5, Y: 0, HP: 100, MaxHP: 100},
				{ID: 2, Type: "tsla", X: 100, Y: 0, HP: 200, MaxHP: 200},
			},
		}}
		got := env.BestGroundTarget()
		if got == nil || got.ID != 1 {
			t.Errorf("expected nearby gun (ID=1), got %+v", got)
		}
	})

	t.Run("damaged target gets bonus", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "gun", X: 10, Y: 0, HP: 100, MaxHP: 100},
				{ID: 2, Type: "gun", X: 10, Y: 0, HP: 20, MaxHP: 100},
			},
		}}
		got := env.BestGroundTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected damaged gun (ID=2), got %+v", got)
		}
	})

	t.Run("faction variant stripping", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "e1", X: 10, Y: 0, HP: 50, MaxHP: 50},
				{ID: 2, Type: "weap.ukraine", X: 10, Y: 0, HP: 100, MaxHP: 100},
			},
		}}
		got := env.BestGroundTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected weap.ukraine (ID=2), got %+v", got)
		}
	})

	t.Run("returns nil when empty", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies:   nil,
		}}
		if got := env.BestGroundTarget(); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("skips MaxHP=0 enemies", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "tsla", X: 10, Y: 0, HP: 0, MaxHP: 0},
			},
		}}
		if got := env.BestGroundTarget(); got != nil {
			t.Errorf("expected nil for MaxHP=0 enemy, got %+v", got)
		}
	})
}
