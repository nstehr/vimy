package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func TestTerrainAtNilGrid(t *testing.T) {
	env := RuleEnv{Terrain: nil}
	if got := env.TerrainAt(10, 10); got != model.Land {
		t.Errorf("TerrainAt with nil grid = %d, want Land", got)
	}
}

func TestTerrainAtWithGrid(t *testing.T) {
	grid := &model.TerrainGrid{
		Cols: 4, Rows: 4, CellW: 8, CellH: 8,
		Grid: []model.TerrainType{
			model.Land, model.Land, model.Water, model.Water,
			model.Land, model.Land, model.Water, model.Water,
			model.Cliff, model.Bridge, model.Land, model.Land,
			model.Cliff, model.Land, model.Land, model.Land,
		},
	}
	env := RuleEnv{Terrain: grid}

	if got := env.TerrainAt(0, 0); got != model.Land {
		t.Errorf("TerrainAt(0,0) = %d, want Land", got)
	}
	if got := env.TerrainAt(16, 0); got != model.Water {
		t.Errorf("TerrainAt(16,0) = %d, want Water", got)
	}
	if got := env.TerrainAt(0, 16); got != model.Cliff {
		t.Errorf("TerrainAt(0,16) = %d, want Cliff", got)
	}
	if got := env.TerrainAt(8, 16); got != model.Bridge {
		t.Errorf("TerrainAt(8,16) = %d, want Bridge", got)
	}
}

func TestIsLandAt(t *testing.T) {
	grid := &model.TerrainGrid{
		Cols: 2, Rows: 2, CellW: 4, CellH: 4,
		Grid: []model.TerrainType{model.Land, model.Water, model.Cliff, model.Bridge},
	}
	env := RuleEnv{Terrain: grid}

	if !env.IsLandAt(0, 0) {
		t.Error("IsLandAt(0,0) should be true for Land")
	}
	if env.IsLandAt(4, 0) {
		t.Error("IsLandAt(4,0) should be false for Water")
	}
	if env.IsLandAt(0, 4) {
		t.Error("IsLandAt(0,4) should be false for Cliff")
	}
	if !env.IsLandAt(4, 4) {
		t.Error("IsLandAt(4,4) should be true for Bridge")
	}
}

func TestIsWaterAt(t *testing.T) {
	grid := &model.TerrainGrid{
		Cols: 2, Rows: 2, CellW: 4, CellH: 4,
		Grid: []model.TerrainType{model.Land, model.Water, model.Cliff, model.Bridge},
	}
	env := RuleEnv{Terrain: grid}

	if env.IsWaterAt(0, 0) {
		t.Error("IsWaterAt(0,0) should be false for Land")
	}
	if !env.IsWaterAt(4, 0) {
		t.Error("IsWaterAt(4,0) should be true for Water")
	}
}

func TestMapHasWaterNilGrid(t *testing.T) {
	env := RuleEnv{Terrain: nil}
	if !env.MapHasWater() {
		t.Error("MapHasWater() with nil grid should return true (assume water possible)")
	}
}

func TestMapHasWater(t *testing.T) {
	noWater := &model.TerrainGrid{
		Cols: 2, Rows: 2, CellW: 4, CellH: 4,
		Grid: []model.TerrainType{model.Land, model.Land, model.Cliff, model.Land},
	}
	env := RuleEnv{Terrain: noWater}
	if env.MapHasWater() {
		t.Error("MapHasWater() should be false for land-only grid")
	}

	withWater := &model.TerrainGrid{
		Cols: 2, Rows: 2, CellW: 4, CellH: 4,
		Grid: []model.TerrainType{model.Land, model.Water, model.Cliff, model.Land},
	}
	env2 := RuleEnv{Terrain: withWater}
	if !env2.MapHasWater() {
		t.Error("MapHasWater() should be true for grid with water")
	}
}

func TestGenerateWaypointsFiltersImpassable(t *testing.T) {
	// 32x32 map, 4x4 cells. Make most zones water to test filtering.
	grid := &model.TerrainGrid{
		Cols: 32, Rows: 32, CellW: 4, CellH: 4,
	}
	grid.Grid = make([]model.TerrainType, 32*32)
	// Set all to water.
	for i := range grid.Grid {
		grid.Grid[i] = model.Water
	}
	// Set center zone to land so at least one waypoint survives.
	// Map center = 64,64 → col=16,row=16
	grid.Grid[16*32+16] = model.Land

	wps := generateWaypoints(128, 128, grid)
	if len(wps) == 0 {
		t.Fatal("generateWaypoints returned no waypoints")
	}
	// Verify all surviving waypoints are on land.
	for _, wp := range wps {
		tt := grid.AtMapPos(wp[0], wp[1])
		if tt != model.Land && tt != model.Bridge {
			t.Errorf("waypoint (%d,%d) is on terrain %d, want Land or Bridge", wp[0], wp[1], tt)
		}
	}
}

func TestGenerateWaypointsNoGrid(t *testing.T) {
	wps := generateWaypoints(128, 128, nil)
	if len(wps) != 9 {
		t.Errorf("generateWaypoints with nil grid returned %d waypoints, want 9", len(wps))
	}
}

func TestGenerateWaypointsFallbackWhenAllFiltered(t *testing.T) {
	// Grid where all waypoint positions are impassable.
	grid := &model.TerrainGrid{
		Cols: 32, Rows: 32, CellW: 4, CellH: 4,
	}
	grid.Grid = make([]model.TerrainType, 32*32)
	for i := range grid.Grid {
		grid.Grid[i] = model.Water
	}

	wps := generateWaypoints(128, 128, grid)
	// Should fallback to all 9 candidates when everything is filtered out.
	if len(wps) != 9 {
		t.Errorf("generateWaypoints with all-water grid returned %d waypoints, want 9 (fallback)", len(wps))
	}
}

func TestDoctrineNavalGating(t *testing.T) {
	// Compile a doctrine with naval weight and verify MapHasWater() appears in conditions.
	d := DefaultDoctrine()
	d.NavalWeight = 0.5
	rules := CompileDoctrine(d)

	navalRuleNames := map[string]bool{
		"build-naval-yard":   true,
		"produce-ship":       true,
		"form-naval-attack":  true,
		"squad-naval-attack": true,
		"rebuild-naval-yard": true,
	}

	for _, r := range rules {
		if navalRuleNames[r.Name] {
			if !containsString(r.ConditionSrc, "MapHasWater()") {
				t.Errorf("rule %q should contain MapHasWater() in condition: %s", r.Name, r.ConditionSrc)
			}
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
