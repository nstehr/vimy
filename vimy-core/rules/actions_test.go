package rules

import (
	"math"
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func TestDefenseHint_NoBuildings(t *testing.T) {
	env := RuleEnv{
		State:  model.GameState{},
		Memory: make(map[string]any),
	}
	x, y := defenseHint(env)
	if x != 0 || y != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", x, y)
	}
}

func TestDefenseHint_SingleBuilding(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100},
			},
		},
		Memory: make(map[string]any),
	}
	// With a single building, radius clamps to 3, and candidates are placed
	// around the perimeter. Result must be near the building.
	for range 20 {
		x, y := defenseHint(env)
		dx := math.Abs(float64(x - 100))
		dy := math.Abs(float64(y - 100))
		if dx > 10 || dy > 10 {
			t.Errorf("hint (%d,%d) too far from single building at (100,100)", x, y)
		}
	}
}

func TestDefenseHint_ThreatBias(t *testing.T) {
	// Enemy base is to the east (X=500). Defenses should bias eastward.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100},
				{ID: 2, Type: "powr", X: 80, Y: 120},
				{ID: 3, Type: "proc", X: 120, Y: 80},
			},
		},
		Memory: make(map[string]any),
	}
	// Seed enemy base intel.
	env.Memory["enemyBases"] = map[string]EnemyBaseIntel{
		"Enemy": {Owner: "Enemy", X: 500, Y: 100, Tick: 1, FromBuildings: true},
	}

	eastCount, westCount := 0, 0
	cx := 100 // approximate centroid
	for range 200 {
		x, _ := defenseHint(env)
		if x > cx {
			eastCount++
		} else {
			westCount++
		}
	}
	// With threat to the east, we expect a strong eastward bias.
	if eastCount <= westCount {
		t.Errorf("expected eastward bias: east=%d west=%d", eastCount, westCount)
	}
}

func TestDefenseHint_SpreadFromExisting(t *testing.T) {
	// Place an existing defense at the north of the base. New defenses
	// should generally avoid clustering there.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100},
				{ID: 2, Type: "powr", X: 80, Y: 120},
				{ID: 3, Type: "powr", X: 120, Y: 120},
				{ID: 4, Type: "pbox", X: 100, Y: 80}, // existing defense, north
			},
		},
		Memory: make(map[string]any),
	}

	northCount, otherCount := 0, 0
	cy := 105 // approximate centroid
	for range 200 {
		_, y := defenseHint(env)
		if y < cy-10 {
			northCount++
		} else {
			otherCount++
		}
	}
	// Spread factor should discourage clustering near the existing defense.
	// Other directions should dominate.
	if northCount > otherCount {
		t.Errorf("expected spread away from existing defense: north=%d other=%d", northCount, otherCount)
	}
}

func TestDefenseHint_AllWaterFallback(t *testing.T) {
	// Terrain grid that is entirely water — should fall back to centroid.
	grid := &model.TerrainGrid{
		Cols:  4,
		Rows:  4,
		CellW: 100,
		CellH: 100,
		Grid:  make([]model.TerrainType, 16),
	}
	for i := range grid.Grid {
		grid.Grid[i] = model.Water
	}

	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 200, Y: 200},
				{ID: 2, Type: "powr", X: 180, Y: 220},
			},
		},
		Memory:  make(map[string]any),
		Terrain: grid,
	}

	x, y := defenseHint(env)
	// Should fall back to centroid (190, 210).
	cx, cy := 190, 210
	dx := math.Abs(float64(x - cx))
	dy := math.Abs(float64(y - cy))
	if dx > 1 || dy > 1 {
		t.Errorf("expected centroid fallback (~%d,~%d), got (%d,%d)", cx, cy, x, y)
	}
}

func TestDefenseHint_NoEnemyIntel(t *testing.T) {
	// Without enemy intel, threat is neutral. Results should still be
	// distributed around the perimeter (no crash, reasonable positions).
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 200, Y: 200},
				{ID: 2, Type: "proc", X: 160, Y: 240},
				{ID: 3, Type: "weap", X: 240, Y: 160},
			},
		},
		Memory: make(map[string]any),
	}

	for range 50 {
		x, y := defenseHint(env)
		// Should be in the general vicinity of the base, not wildly off.
		dx := math.Abs(float64(x - 200))
		dy := math.Abs(float64(y - 200))
		if dx > 200 || dy > 200 {
			t.Errorf("hint (%d,%d) unreasonably far from base centroid", x, y)
		}
	}
}
