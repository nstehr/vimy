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
			"retreatingUnits": map[int]int{1: 0},
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

func TestOverextendedSquadMembers_ExemptsForwardStaging(t *testing.T) {
	// Our base at (100,100). Enemy base at (900,900). Unit 2 at (550,550) — a
	// flank-waypoint staging position halfway between bases, closer to enemy
	// than to home. Must NOT be flagged overextended (vimy-rmb).
	env := RuleEnv{
		State: model.GameState{
			MapWidth:  1000,
			MapHeight: 1000,
			Buildings: []model.Building{{ID: 100, Type: "fact", X: 100, Y: 100}},
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 100, Y: 100, Idle: true}, // near our base
				{ID: 2, Type: "3tnk", X: 550, Y: 550, Idle: true}, // staging closer to enemy
				{ID: 3, Type: "3tnk", X: 100, Y: 950, Idle: true}, // wandered away (far home, far enemy)
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{1, 2, 3}, TargetSize: 3},
			},
			"enemyBases": map[string]EnemyBaseIntel{
				"opp": {Owner: "opp", X: 900, Y: 900, Tick: 100},
			},
		},
	}

	got := env.OverextendedSquadMembers("ground-attack", 0.25)
	// Unit 2 is closer to enemy than to home → forward-progressing → exempt.
	// Unit 3 is at (100,950): dist-to-home ≈ 850, dist-to-enemy ≈ 802 — also
	// closer to enemy. Exempt. So only "no-man's-land OFF-axis" should flag.
	// Update unit 3 to a true no-man's-land position perpendicular to the axis.
	for _, u := range got {
		if u.ID == 2 {
			t.Errorf("unit 2 (forward staging) should NOT be flagged, got %v", got)
		}
	}
}

func TestOverextendedSquadMembers_FlagsOffAxisWanderer(t *testing.T) {
	// Off-axis: home at (100,100), enemy at (900,100). Unit way north at
	// (500, 950) — far from both bases AND not making forward progress
	// (distance to enemy ≈ 950, distance to home ≈ 940 — slightly closer to
	// home so NOT exempt by the forward-progress rule).
	env := RuleEnv{
		State: model.GameState{
			MapWidth:  1000,
			MapHeight: 1000,
			Buildings: []model.Building{{ID: 100, Type: "fact", X: 100, Y: 100}},
			Units: []model.Unit{
				{ID: 7, Type: "3tnk", X: 500, Y: 950, Idle: true},
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{7}, TargetSize: 1},
			},
			"enemyBases": map[string]EnemyBaseIntel{
				"opp": {Owner: "opp", X: 900, Y: 100, Tick: 100},
			},
		},
	}
	got := env.OverextendedSquadMembers("ground-attack", 0.25)
	if len(got) != 1 || got[0].ID != 7 {
		t.Errorf("expected off-axis wanderer to be flagged, got %v", got)
	}
}

func TestOverextendedSquadMembers_ExemptsUnitsNearEnemyBase(t *testing.T) {
	// Unit 2 is far from our base (would normally be flagged) but adjacent to
	// a known enemy base — it's correctly forward-deployed and must not be
	// recalled (vimy-91b).
	env := RuleEnv{
		State: model.GameState{
			MapWidth:  1000,
			MapHeight: 1000,
			Buildings: []model.Building{{ID: 100, Type: "fact", X: 100, Y: 100}},
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 100, Y: 100, Idle: true}, // near base
				{ID: 2, Type: "3tnk", X: 900, Y: 900, Idle: true}, // AT enemy base
				{ID: 3, Type: "3tnk", X: 100, Y: 600, Idle: true}, // off-axis (perpendicular to home-enemy line)
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{1, 2, 3}, TargetSize: 3},
			},
			"enemyBases": map[string]EnemyBaseIntel{
				"opp": {Owner: "opp", X: 900, Y: 900, Tick: 100},
			},
		},
	}

	got := env.OverextendedSquadMembers("ground-attack", 0.25)
	// Unit 2 (at enemy base) is exempt via forward-progress. Unit 3 is off
	// the home-enemy axis (dist-to-home=500, dist-to-enemy≈854) — closer to
	// home so NOT forward-progressing, still flagged. Unit 1 is inside leash.
	if len(got) != 1 || got[0].ID != 3 {
		t.Errorf("expected only unit 3 (off-axis wanderer) to be flagged, got %v", got)
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

func TestBestGroundTarget_AABoosted(t *testing.T) {
	base := model.Building{X: 0, Y: 0}

	t.Run("AA beats power plant with bias", func(t *testing.T) {
		// Without bias: AA=2, powr=2 → tie (both same distance).
		// With GroundAA=3.0: AA=2*3=6, powr=2 → AA wins.
		env := RuleEnv{
			State: model.GameState{
				Buildings: []model.Building{base},
				Enemies: []model.Enemy{
					{ID: 1, Type: "powr", X: 10, Y: 0, HP: 100, MaxHP: 100},
					{ID: 2, Type: "agun", X: 10, Y: 0, HP: 100, MaxHP: 100},
				},
			},
			TargetBias: TargetBias{GroundAA: 3.0},
		}
		got := env.BestGroundTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected agun (ID=2) with AA bias, got %+v", got)
		}
	})

	t.Run("SAM site boosted with bias", func(t *testing.T) {
		env := RuleEnv{
			State: model.GameState{
				Buildings: []model.Building{base},
				Enemies: []model.Enemy{
					{ID: 1, Type: "powr", X: 10, Y: 0, HP: 100, MaxHP: 100},
					{ID: 2, Type: "sam", X: 10, Y: 0, HP: 100, MaxHP: 100},
				},
			},
			TargetBias: TargetBias{GroundAA: 3.0},
		}
		got := env.BestGroundTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected sam (ID=2) with AA bias, got %+v", got)
		}
	})

	t.Run("no bias preserves default behavior", func(t *testing.T) {
		// Without bias, tesla (10) beats AA (2) at same distance.
		env := RuleEnv{
			State: model.GameState{
				Buildings: []model.Building{base},
				Enemies: []model.Enemy{
					{ID: 1, Type: "agun", X: 10, Y: 0, HP: 100, MaxHP: 100},
					{ID: 2, Type: "tsla", X: 10, Y: 0, HP: 200, MaxHP: 200},
				},
			},
		}
		got := env.BestGroundTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected tsla (ID=2) without bias, got %+v", got)
		}
	})
}

func TestBestAirTarget_GroundDefBoosted(t *testing.T) {
	base := model.Building{X: 0, Y: 0}

	t.Run("pillbox boosted over refinery with bias", func(t *testing.T) {
		// With AirGroundDef=3.0: pbox=7*3=21 vs proc=6 → strong preference.
		// pbox at dist=20: score = 21 * 1.0 / sqrt(20) ≈ 4.70
		// proc at dist=5:  score =  6 * 1.0 / sqrt(5)  ≈ 2.68
		// (Construction yard / fact is intentionally NOT used here — its
		// post-vimy-68x value of 12 is high enough that a 3x bias on a
		// neighbouring pillbox can't overcome it at close range, which is
		// the new and correct behaviour.)
		env := RuleEnv{
			State: model.GameState{
				Buildings: []model.Building{base},
				Enemies: []model.Enemy{
					{ID: 1, Type: "proc", X: 5, Y: 0, HP: 100, MaxHP: 100},
					{ID: 2, Type: "pbox", X: 20, Y: 0, HP: 100, MaxHP: 100},
				},
			},
			TargetBias: TargetBias{AirGroundDef: 3.0},
		}
		got := env.BestAirTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected pbox (ID=2) with ground def bias, got %+v", got)
		}
	})

	t.Run("tesla boosted with bias", func(t *testing.T) {
		env := RuleEnv{
			State: model.GameState{
				Buildings: []model.Building{base},
				Enemies: []model.Enemy{
					{ID: 1, Type: "mslo", X: 10, Y: 0, HP: 200, MaxHP: 200}, // val=9
					{ID: 2, Type: "tsla", X: 10, Y: 0, HP: 200, MaxHP: 200}, // val=10, boosted to 30
				},
			},
			TargetBias: TargetBias{AirGroundDef: 3.0},
		}
		got := env.BestAirTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected tsla (ID=2) with ground def bias, got %+v", got)
		}
	})
}

func TestBestCapturable_SkipsWater(t *testing.T) {
	grid := &model.TerrainGrid{
		Cols: 4, Rows: 4, CellW: 100, CellH: 100,
		Grid: []model.TerrainType{
			model.Land, model.Land, model.Water, model.Water,
			model.Land, model.Land, model.Water, model.Water,
			model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land,
		},
	}

	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 50, Y: 50},
			},
			Capturables: []model.Enemy{
				{ID: 10, Type: "oilb", X: 250, Y: 50},  // on water — should be skipped
				{ID: 11, Type: "hosp", X: 150, Y: 150},  // on land — should be picked
			},
		},
		Terrain: grid,
	}

	got := env.BestCapturable()
	if got == nil {
		t.Fatal("expected a capturable, got nil")
	}
	if got.ID != 11 {
		t.Errorf("expected land-based capturable ID=11, got ID=%d", got.ID)
	}
}

func TestBestCapturable_NoTerrainGrid(t *testing.T) {
	// Without terrain grid, all capturables should be considered.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 50, Y: 50},
			},
			Capturables: []model.Enemy{
				{ID: 10, Type: "oilb", X: 60, Y: 50},
			},
		},
		Terrain: nil,
	}

	got := env.BestCapturable()
	if got == nil || got.ID != 10 {
		t.Errorf("expected capturable ID=10 without terrain filter, got %+v", got)
	}
}

func TestUpdateIntel_ClearsStaleIntel(t *testing.T) {
	// Intel at tick 200. First eligible-to-clear tick is 200+3000=3200.
	// After that, requires 500 ticks of continuous "our unit near + no enemy"
	// before clearing (vimy scout-clearing-intel fix).
	env := RuleEnv{
		State: model.GameState{
			Tick: 3500,
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 100, Y: 100}, // our unit near intel position
			},
			Enemies: []model.Enemy{},
		},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy1": {Owner: "Enemy1", X: 105, Y: 105, Tick: 200, FromBuildings: true},
			},
		},
	}

	// First call: starts the sustain counter, doesn't clear yet.
	updateIntel(env)
	if _, exists := getEnemyBases(env.Memory)["Enemy1"]; !exists {
		t.Fatal("intel should NOT clear on first tick — needs sustained confirmation")
	}

	// Advance past the sustain threshold with our unit still parked there.
	env.State.Tick = 4100 // 3500 + 600 > intelClearSustain (500)
	updateIntel(env)
	if _, exists := getEnemyBases(env.Memory)["Enemy1"]; exists {
		t.Error("expected stale intel for Enemy1 to be cleared after sustained confirmation")
	}
}

func TestUpdateIntel_ScoutPassThroughDoesNotClear(t *testing.T) {
	// Scout is briefly near the enemy base and no enemy visible at that
	// instant. Intel must NOT clear — otherwise a scout patrol wipes the
	// enemy base intel every rotation (observed live in vimy match).
	env := RuleEnv{
		State: model.GameState{
			Tick: 3500,
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 100, Y: 100},
			},
			Enemies: []model.Enemy{},
		},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy1": {Owner: "Enemy1", X: 105, Y: 105, Tick: 200, FromBuildings: true},
			},
		},
	}
	updateIntel(env)
	if _, exists := getEnemyBases(env.Memory)["Enemy1"]; !exists {
		t.Fatal("intel cleared on first tick — sustained confirmation broken")
	}
	// Scout moves away next tick.
	env.State.Tick = 3510
	env.State.Units[0].X = 500
	env.State.Units[0].Y = 500
	updateIntel(env)
	if _, exists := getEnemyBases(env.Memory)["Enemy1"]; !exists {
		t.Error("intel cleared after scout moved away — should still persist")
	}
}

// InferredEnemyBaseCenter should combine currently-visible buildings,
// visible harvesters, persistent harvester sightings, and the existing
// high-confidence base centroid into a single weighted point. Buildings
// pull strongly; harvester-only sightings return a lower-confidence result.
func TestInferredEnemyBaseCenter_NoIntelReturnsNil(t *testing.T) {
	env := RuleEnv{
		State:  model.GameState{Tick: 100},
		Memory: map[string]any{},
	}
	if env.InferredEnemyBaseCenter() != nil {
		t.Error("expected nil inferred center when no intel exists")
	}
}

func TestInferredEnemyBaseCenter_HarvestersAlone(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Tick: 100,
			Enemies: []model.Enemy{
				{ID: 1, Type: "harv", Owner: "Enemy", X: 80, Y: 80},
			},
		},
		Memory: map[string]any{},
	}
	center := env.InferredEnemyBaseCenter()
	if center == nil {
		t.Fatal("expected non-nil center from harvester sighting")
	}
	if center.X != 80 || center.Y != 80 {
		t.Errorf("expected center at harvester position (80,80), got (%d,%d)", center.X, center.Y)
	}
	if center.Confidence >= 0.5 {
		t.Errorf("single harvester should yield low confidence, got %.2f", center.Confidence)
	}
}

func TestInferredEnemyBaseCenter_BuildingsPullStronger(t *testing.T) {
	// A building at (100,100) should pull the centroid much harder than a
	// distant harvester at (10,10). Expected centroid near (100,100), not
	// halfway.
	env := RuleEnv{
		State: model.GameState{
			Tick: 100,
			Enemies: []model.Enemy{
				{ID: 1, Type: "proc", Owner: "Enemy", X: 100, Y: 100},
				{ID: 2, Type: "harv", Owner: "Enemy", X: 10, Y: 10},
			},
		},
		Memory: map[string]any{},
	}
	center := env.InferredEnemyBaseCenter()
	if center == nil {
		t.Fatal("expected center")
	}
	// Building weight 3 at (100,100) + harvester weight 1 at (10,10) → weighted X = (300+10)/4 = 77.5
	if center.X < 70 || center.X > 85 {
		t.Errorf("expected centroid pulled toward building (x≈77), got %d", center.X)
	}
}

func TestInferredEnemyBaseCenter_PersistentHarvesterIntel(t *testing.T) {
	// Harvester sighted 100 ticks ago, no longer visible. The persistent
	// intel should still contribute to the centroid.
	env := RuleEnv{
		State: model.GameState{
			Tick:    200,
			Enemies: []model.Enemy{}, // no current sightings
		},
		Memory: map[string]any{
			"enemyHarvesterIntel": map[int]enemyHarvesterIntel{
				7: {X: 60, Y: 60, Tick: 100},
			},
		},
	}
	center := env.InferredEnemyBaseCenter()
	if center == nil {
		t.Fatal("persistent harvester intel should yield a center")
	}
	if center.X != 60 || center.Y != 60 {
		t.Errorf("expected center at persisted harvester position, got (%d,%d)", center.X, center.Y)
	}
}

func TestInferredEnemyBaseCenter_DecaysOldHarvesterIntel(t *testing.T) {
	// One recent harvester at (50,50), one very old (600 ticks ago) at
	// (0,0). Old one should count at half weight; centroid skews toward the
	// recent sighting.
	env := RuleEnv{
		State: model.GameState{Tick: 800},
		Memory: map[string]any{
			"enemyHarvesterIntel": map[int]enemyHarvesterIntel{
				1: {X: 50, Y: 50, Tick: 800}, // fresh, weight 1.0
				2: {X: 0, Y: 0, Tick: 200},   // 600 ticks old → weight 0.5
			},
		},
	}
	center := env.InferredEnemyBaseCenter()
	if center == nil {
		t.Fatal("expected center")
	}
	// Weighted X = (50*1 + 0*0.5) / 1.5 ≈ 33
	if center.X < 28 || center.X > 38 {
		t.Errorf("expected decayed-weight centroid near x≈33, got %d", center.X)
	}
}

func TestUpdateIntel_PersistsHarvesterSightings(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Tick: 500,
			Enemies: []model.Enemy{
				{ID: 42, Type: "harv", Owner: "Enemy", X: 123, Y: 45},
			},
		},
		Memory: make(map[string]any),
	}
	updateIntel(env)

	harvesters := memoryMap[int, enemyHarvesterIntel](env.Memory, "enemyHarvesterIntel")
	got, ok := harvesters[42]
	if !ok {
		t.Fatal("expected harvester 42 to be persisted after updateIntel")
	}
	if got.X != 123 || got.Y != 45 || got.Tick != 500 {
		t.Errorf("expected {123,45,500}, got %+v", got)
	}
}

func TestUpdateIntel_KeepsFreshIntel(t *testing.T) {
	// Intel is only 100 ticks old (< 300 threshold) — should not be cleared
	// even though our units are at the position and no enemies visible.
	env := RuleEnv{
		State: model.GameState{
			Tick: 300,
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 100, Y: 100},
			},
			Enemies: []model.Enemy{},
		},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy1": {Owner: "Enemy1", X: 105, Y: 105, Tick: 200, FromBuildings: true},
			},
		},
	}

	updateIntel(env)

	bases := getEnemyBases(env.Memory)
	if _, exists := bases["Enemy1"]; !exists {
		t.Error("expected fresh intel for Enemy1 to be kept (age 100 < 300 threshold)")
	}
}

func TestUpdateIntel_KeepsIntelWhenEnemiesNearby(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Tick: 600,
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 100, Y: 100}, // our unit near intel
			},
			Enemies: []model.Enemy{
				{ID: 10, Owner: "Enemy1", Type: "tsla", X: 103, Y: 103, HP: 100, MaxHP: 100}, // enemy still there
			},
		},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy1": {Owner: "Enemy1", X: 105, Y: 105, Tick: 200, FromBuildings: true},
			},
		},
	}

	updateIntel(env)

	bases := getEnemyBases(env.Memory)
	if _, exists := bases["Enemy1"]; !exists {
		t.Error("expected intel for Enemy1 to be kept when enemies are nearby")
	}
}

func TestUpdateIntel_KeepsIntelWhenNoOwnUnitsNearby(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Tick: 600,
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", X: 500, Y: 500}, // our unit far from intel
			},
			Enemies: []model.Enemy{},
		},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy1": {Owner: "Enemy1", X: 100, Y: 100, Tick: 200, FromBuildings: true},
			},
		},
	}

	updateIntel(env)

	bases := getEnemyBases(env.Memory)
	if _, exists := bases["Enemy1"]; !exists {
		t.Error("expected intel for Enemy1 to be kept when no own units nearby")
	}
}

func TestServiceDepot(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100},
				{ID: 2, Type: "fix", X: 200, Y: 300},
			},
		},
	}
	depot := env.ServiceDepot()
	if depot == nil {
		t.Fatal("expected service depot, got nil")
	}
	if depot.ID != 2 {
		t.Errorf("expected depot ID=2, got %d", depot.ID)
	}
}

func TestServiceDepot_None(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100},
			},
		},
	}
	if env.ServiceDepot() != nil {
		t.Error("expected nil when no service depot")
	}
}

func TestChokepointsTowardEnemy_RanksByPath(t *testing.T) {
	// Two bridges. Base at map (50,50) → zone (0,0). Enemy at (450,450) →
	// zone (4,4). The bridge at (2,1) lies on the shortest passable path;
	// the bridge at (0,3) is reachable but off the direct route.
	grid := &model.TerrainGrid{
		Cols: 5, Rows: 5, CellW: 100, CellH: 100,
		Grid: []model.TerrainType{
			model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Water, model.Water, model.Bridge, model.Water, model.Water,
			model.Water, model.Land, model.Land, model.Land, model.Water,
			model.Bridge, model.Land, model.Land, model.Land, model.Water,
			model.Land, model.Land, model.Land, model.Land, model.Land,
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{{ID: 1, Type: "fact", X: 50, Y: 50}},
		},
		Terrain: grid,
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy1": {Owner: "Enemy1", X: 450, Y: 450, Tick: 1, FromBuildings: true},
			},
		},
	}

	got := env.ChokepointsTowardEnemy()
	if len(got) == 0 {
		t.Fatal("expected ranked chokepoints, got none")
	}
	if got[0].Col != 2 || got[0].Row != 1 {
		t.Errorf("top choke = (%d,%d), want on-path bridge (2,1)", got[0].Col, got[0].Row)
	}
}

func TestChokepointsTowardEnemy_CachesDetection(t *testing.T) {
	grid := &model.TerrainGrid{
		Cols: 4, Rows: 4, CellW: 10, CellH: 10,
		Grid: []model.TerrainType{
			model.Land, model.Land, model.Water, model.Water,
			model.Land, model.Land, model.Water, model.Water,
			model.Cliff, model.Bridge, model.Land, model.Land,
			model.Cliff, model.Land, model.Land, model.Land,
		},
	}
	mem := map[string]any{}
	env := RuleEnv{Terrain: grid, Memory: mem}

	_ = env.ChokepointsTowardEnemy()
	cached, ok := mem["chokepoints"].([]model.Chokepoint)
	if !ok {
		t.Fatalf("expected chokepoints cached in memory, got %T", mem["chokepoints"])
	}
	if len(cached) != 1 {
		t.Errorf("expected 1 cached choke, got %d", len(cached))
	}
}

func TestChokepointsTowardEnemy_NoTerrain(t *testing.T) {
	env := RuleEnv{Memory: map[string]any{}}
	if got := env.ChokepointsTowardEnemy(); got != nil {
		t.Errorf("expected nil without terrain, got %+v", got)
	}
}

func TestApproachWaypoint_RoutesAroundThreat(t *testing.T) {
	// 7x5 open land. Base at (50,50), enemy base at (650,250). Remembered
	// pillboxes block the direct middle-row corridor, forcing the waypoint
	// to skirt north or south instead of charging through.
	grid := &model.TerrainGrid{
		Cols: 7, Rows: 5, CellW: 100, CellH: 100,
		Grid: []model.TerrainType{
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Tick:      500,
			Buildings: []model.Building{{ID: 1, Type: "fact", X: 50, Y: 50}},
		},
		Memory: map[string]any{
			"enemyDefenses": map[int]EnemyDefenseIntel{
				10: {ActorID: 10, Type: Pillbox, X: 350, Y: 250, Tick: 100},
				11: {ActorID: 11, Type: Pillbox, X: 450, Y: 250, Tick: 100},
			},
		},
		Terrain: grid,
	}

	wx, wy, ok := env.ApproachWaypoint(650, 250)
	if !ok {
		t.Fatal("expected a waypoint when direct path is contested")
	}
	// The waypoint should not be in the middle row (row=2, y=200..299).
	if wy >= 200 && wy < 300 {
		// Acceptable only if it's pre-threat zone before the pillboxes (x < 300).
		if wx >= 300 {
			t.Errorf("waypoint (%d,%d) should skirt threat row, not pass through it", wx, wy)
		}
	}
}

func TestAirApproachWaypoint_RoutesAroundAA(t *testing.T) {
	// 7x5 open land. Base at (50,50), target at (650,250). SAM cluster blocks
	// the middle row. A pillbox in the same row should NOT influence routing
	// (only AA matters for aircraft).
	grid := &model.TerrainGrid{
		Cols: 7, Rows: 5, CellW: 100, CellH: 100,
		Grid: []model.TerrainType{
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Tick:      500,
			Buildings: []model.Building{{ID: 1, Type: "fact", X: 50, Y: 50}},
		},
		Memory: map[string]any{
			"enemyDefenses": map[int]EnemyDefenseIntel{
				10: {ActorID: 10, Type: SAMSite, X: 350, Y: 250, Tick: 100},
				11: {ActorID: 11, Type: SAMSite, X: 450, Y: 250, Tick: 100},
			},
		},
		Terrain: grid,
	}

	wx, wy, ok := env.AirApproachWaypoint(650, 250)
	if !ok {
		t.Fatal("expected an air waypoint when SAM cluster blocks direct path")
	}
	if wy >= 200 && wy < 300 && wx >= 300 {
		t.Errorf("air waypoint (%d,%d) should skirt SAM row, not pass through it", wx, wy)
	}
}

func TestAirApproachWaypoint_IgnoresGroundDefenses(t *testing.T) {
	// Pillboxes alone should NOT trigger an air waypoint — they don't shoot
	// aircraft, so the air corridor is open.
	grid := &model.TerrainGrid{
		Cols: 7, Rows: 5, CellW: 100, CellH: 100,
		Grid: make([]model.TerrainType, 35),
	}
	env := RuleEnv{
		State: model.GameState{
			Tick:      500,
			Buildings: []model.Building{{ID: 1, Type: "fact", X: 50, Y: 50}},
		},
		Memory: map[string]any{
			"enemyDefenses": map[int]EnemyDefenseIntel{
				10: {ActorID: 10, Type: Pillbox, X: 350, Y: 250, Tick: 100},
				11: {ActorID: 11, Type: TeslaCoil, X: 450, Y: 250, Tick: 100},
			},
		},
		Terrain: grid,
	}
	if _, _, ok := env.AirApproachWaypoint(650, 250); ok {
		t.Error("air waypoint should not trigger on ground defenses alone")
	}
}

func TestBestApproachAxis_PicksOpenFlank(t *testing.T) {
	// Base at southeast (650, 450). Enemy target at northwest (250, 150).
	// Defenses clustered SOUTH of the target (250, 250..280). EAST of target
	// (450, 150) is wide open. BestApproachAxis must pick a waypoint on the
	// east/north side of the target, not somewhere south of it where the
	// defenses are painted.
	grid := &model.TerrainGrid{
		Cols: 9, Rows: 7, CellW: 100, CellH: 100,
		Grid: make([]model.TerrainType, 9*7),
	}
	for i := range grid.Grid {
		grid.Grid[i] = model.Land
	}
	env := RuleEnv{
		State: model.GameState{
			Tick:      500,
			MapWidth:  900,
			MapHeight: 700,
			Buildings: []model.Building{{ID: 1, Type: "fact", X: 650, Y: 450}},
		},
		Memory: map[string]any{
			"enemyDefenses": map[int]EnemyDefenseIntel{
				10: {ActorID: 10, Type: Pillbox, X: 250, Y: 280, Tick: 100},
				11: {ActorID: 11, Type: Pillbox, X: 300, Y: 280, Tick: 100},
				12: {ActorID: 12, Type: TeslaCoil, X: 200, Y: 280, Tick: 100},
			},
		},
		Terrain: grid,
	}

	wx, wy, ok := env.BestApproachAxis(250, 150)
	if !ok {
		t.Fatal("expected BestApproachAxis to pick a flank waypoint")
	}
	// The defenses are on the south side of the target (y around 250-280).
	// A flank waypoint must NOT be south of the target — it should be at or
	// above the target's y (north or east).
	if wy > 150 {
		t.Errorf("waypoint (%d,%d) is south of target — should pick north/east flank away from defenses", wx, wy)
	}
}

func TestAircraftCapacity(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{Type: "afld"}, // Airfield
				{Type: "hpad"}, // Helipad
				{Type: "hpad"}, // Helipad
				{Type: "fact"}, // unrelated
			},
		},
	}
	got := env.AircraftCapacity()
	want := 4 + 1 + 1 // 1 airfield (4 pads) + 2 helipads
	if got != want {
		t.Errorf("AircraftCapacity = %d, want %d", got, want)
	}
}

func TestIsRushedAndIsHarvesterHarassed(t *testing.T) {
	env := RuleEnv{Memory: map[string]any{}}
	if env.IsRushed() || env.IsHarvesterHarassed() {
		t.Error("expected both flags false when memory empty")
	}
	env.Memory["beingRushed"] = true
	if !env.IsRushed() {
		t.Error("expected IsRushed() = true after setting memory")
	}
	if env.IsHarvesterHarassed() {
		t.Error("expected IsHarvesterHarassed() still false")
	}
	env.Memory["harvesterHarassed"] = true
	if !env.IsHarvesterHarassed() {
		t.Error("expected IsHarvesterHarassed() = true")
	}
}

func TestCriticalBuildingUnderAttack_DetectsEnemyNearCY(t *testing.T) {
	// Enemy rocket launcher 5 cells from CY — well within 10-cell attack range.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100, HP: 800, MaxHP: 1000}, // damaged CY
			},
			Enemies: []model.Enemy{
				{ID: 42, Type: "v2rl", X: 105, Y: 105, HP: 100, MaxHP: 150},
			},
		},
	}
	if !env.CriticalBuildingUnderAttack() {
		t.Error("expected CriticalBuildingUnderAttack() = true with enemy 7 units from CY")
	}
}

func TestCriticalBuildingUnderAttack_IgnoresDistantEnemies(t *testing.T) {
	// Enemy 50 cells away — outside attack range.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100, HP: 1000, MaxHP: 1000},
			},
			Enemies: []model.Enemy{
				{ID: 42, Type: "3tnk", X: 200, Y: 200, HP: 400, MaxHP: 400},
			},
		},
	}
	if env.CriticalBuildingUnderAttack() {
		t.Error("expected false when enemy is far from any critical building")
	}
}

func TestCriticalBuildingUnderAttack_IgnoresNearNoncriticalBuildings(t *testing.T) {
	// Enemy near a radar dome (non-critical) — should not trigger.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "dome", X: 100, Y: 100, HP: 500, MaxHP: 1000},
			},
			Enemies: []model.Enemy{
				{ID: 42, Type: "3tnk", X: 102, Y: 100, HP: 400, MaxHP: 400},
			},
		},
	}
	if env.CriticalBuildingUnderAttack() {
		t.Error("expected false when enemy is near a non-critical building only")
	}
}

func TestCriticalBuildingUnderAttack_IgnoresHuskEnemies(t *testing.T) {
	// heli.husk (dead helicopter wreckage) briefly persists in State.Enemies
	// after the unit dies. Should NOT trigger defense engagement.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100, HP: 1000, MaxHP: 1000},
			},
			Enemies: []model.Enemy{
				{ID: 42, Type: "heli.husk", X: 105, Y: 105, HP: 1, MaxHP: 500},
			},
		},
	}
	if env.CriticalBuildingUnderAttack() {
		t.Error("expected false when nearby enemy is a husk (wreckage)")
	}
}

func TestCriticalBuildingUnderAttack_IgnoresEnemyHarvester(t *testing.T) {
	// Enemy harvester near CY — not an attacker.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100, HP: 1000, MaxHP: 1000},
			},
			Enemies: []model.Enemy{
				{ID: 42, Type: "harv", X: 105, Y: 105, HP: 800, MaxHP: 800},
			},
		},
	}
	if env.CriticalBuildingUnderAttack() {
		t.Error("expected false when the nearby enemy is just a harvester")
	}
}

func TestSquadClumped_TightGroup(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, X: 100, Y: 100},
				{ID: 2, X: 102, Y: 100},
				{ID: 3, X: 100, Y: 103},
				{ID: 4, X: 101, Y: 101},
				{ID: 5, X: 99, Y: 99},
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{1, 2, 3, 4, 5}, TargetSize: 5},
			},
		},
	}
	if !env.SquadClumped("ground-attack", 8) {
		t.Error("expected clumped when all units within 8 cells of centroid")
	}
}

func TestSquadClumped_Dispersed(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, X: 100, Y: 100},
				{ID: 2, X: 300, Y: 100}, // 200 cells away
				{ID: 3, X: 500, Y: 200}, // even further
				{ID: 4, X: 700, Y: 300}, // further still
				{ID: 5, X: 100, Y: 100},
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{1, 2, 3, 4, 5}, TargetSize: 5},
			},
		},
	}
	if env.SquadClumped("ground-attack", 8) {
		t.Error("expected NOT clumped when units are hundreds of cells apart")
	}
}

func TestSquadClumped_SingletonTriviallyClumped(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{Units: []model.Unit{{ID: 1, X: 100, Y: 100}}},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{1}, TargetSize: 1},
			},
		},
	}
	if !env.SquadClumped("ground-attack", 8) {
		t.Error("expected single-unit squad to be trivially clumped")
	}
}

func TestSquadClumped_MissingSquadNotAProblem(t *testing.T) {
	env := RuleEnv{Memory: map[string]any{}}
	if !env.SquadClumped("nonexistent", 8) {
		t.Error("expected true (no dispersion to worry about) for missing squad")
	}
}

func TestAxisBurned(t *testing.T) {
	env := RuleEnv{Memory: map[string]any{}}
	if env.AxisBurned("air") {
		t.Error("expected no burn when memory empty")
	}
	env.Memory["burnedAxes"] = map[string]bool{"air": true}
	if !env.AxisBurned("air") {
		t.Error("expected air burned")
	}
	if env.AxisBurned("vehicle") {
		t.Error("expected vehicle not burned")
	}
}

func TestBestApproachAxis_OpenCorridorReturnsNoWaypoint(t *testing.T) {
	// No defenses → direct corridor is clean → no waypoint.
	grid := &model.TerrainGrid{
		Cols: 5, Rows: 5, CellW: 100, CellH: 100,
		Grid: make([]model.TerrainType, 25),
	}
	env := RuleEnv{
		State: model.GameState{
			MapWidth:  500,
			MapHeight: 500,
			Buildings: []model.Building{{ID: 1, Type: "fact", X: 50, Y: 50}},
		},
		Memory:  map[string]any{},
		Terrain: grid,
	}
	if _, _, ok := env.BestApproachAxis(450, 450); ok {
		t.Error("expected no waypoint when corridor is open")
	}
}

func TestApproachWaypoint_NoThreatReturnsNoWaypoint(t *testing.T) {
	grid := &model.TerrainGrid{
		Cols: 5, Rows: 5, CellW: 100, CellH: 100,
		Grid: make([]model.TerrainType, 25),
	}
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{{ID: 1, Type: "fact", X: 50, Y: 50}},
		},
		Memory:  map[string]any{},
		Terrain: grid,
	}
	if _, _, ok := env.ApproachWaypoint(450, 450); ok {
		t.Error("expected no waypoint when no threat intel exists")
	}
}

func TestUpdateDefenseIntel_RemembersVisible(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Tick: 100,
			Enemies: []model.Enemy{
				{ID: 42, Type: "pbox", X: 200, Y: 200, HP: 50, MaxHP: 50},
				{ID: 43, Type: "tsla", X: 220, Y: 220, HP: 100, MaxHP: 100},
				{ID: 44, Type: "e1", X: 250, Y: 250, HP: 100, MaxHP: 100}, // infantry — skip
			},
		},
		Memory: map[string]any{},
	}
	updateDefenseIntel(env)

	defs := getEnemyDefenses(env.Memory)
	if len(defs) != 2 {
		t.Fatalf("expected 2 defenses remembered, got %d (%+v)", len(defs), defs)
	}
	if _, ok := defs[42]; !ok {
		t.Error("expected pillbox ID=42 remembered")
	}
	if _, ok := defs[43]; !ok {
		t.Error("expected tesla ID=43 remembered")
	}
}
