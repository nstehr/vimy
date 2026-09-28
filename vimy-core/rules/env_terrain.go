package rules

import (
	"github.com/nstehr/vimy/vimy-core/model"
)

func (e RuleEnv) MapWidth() int { return e.State.MapWidth }

func (e RuleEnv) MapHeight() int { return e.State.MapHeight }

// TerrainAt resolves map coordinates against the coarse grid, defaulting to
// Land when no grid is available.
func (e RuleEnv) TerrainAt(mapX, mapY int) model.TerrainType {
	if e.Terrain == nil {
		return model.Land
	}
	return e.Terrain.AtMapPos(mapX, mapY)
}

// IsLandAt returns true if the map position is passable ground.
func (e RuleEnv) IsLandAt(mapX, mapY int) bool {
	t := e.TerrainAt(mapX, mapY)
	return t == model.Land || t == model.Bridge
}

// IsWaterAt returns true if the map position is water.
func (e RuleEnv) IsWaterAt(mapX, mapY int) bool {
	return e.TerrainAt(mapX, mapY) == model.Water
}

// MapHasWater is false without a terrain grid — better than gating naval
// production on data we don't have.
func (e RuleEnv) MapHasWater() bool {
	if e.Terrain == nil {
		return true // assume water possible when no terrain data
	}
	return e.Terrain.HasWater()
}

// allChokepoints caches for the match: the terrain grid is static.
func (e RuleEnv) allChokepoints() []model.Chokepoint {
	if e.Terrain == nil {
		return nil
	}
	if cached, ok := e.Memory["chokepoints"].([]model.Chokepoint); ok {
		return cached
	}
	cps := model.FindChokepoints(e.Terrain)
	e.Memory["chokepoints"] = cps
	return cps
}

// ChokepointsTowardEnemy ranks chokepoints by relevance to the path from our
// base to the nearest known enemy base, falling back to the unranked list with
// no intel so callers can still mine on structure alone.
func (e RuleEnv) ChokepointsTowardEnemy() []model.Chokepoint {
	cps := e.allChokepoints()
	if len(cps) == 0 || e.Terrain == nil || e.Terrain.CellW <= 0 || e.Terrain.CellH <= 0 {
		return cps
	}
	base := e.NearestEnemyBase()
	if base == nil {
		return cps
	}
	centX, centY := e.BuildingCentroid()
	from := [2]int{centX / e.Terrain.CellW, centY / e.Terrain.CellH}
	to := [2]int{base.X / e.Terrain.CellW, base.Y / e.Terrain.CellH}
	return model.RankChokepointsOnPath(cps, e.Terrain, from, to)
}

func (e RuleEnv) EnemiesVisible() bool { return len(e.State.Enemies) > 0 }
