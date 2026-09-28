package rules

import (
	"maps"
	"slices"

	"github.com/nstehr/vimy/vimy-core/model"
)

// IntelSnapshot contains owned copies; telemetry and dashboards can inspect it
// without holding the engine's memory lock or retaining mutable maps.
type IntelSnapshot struct {
	Bases         map[string]EnemyBaseIntel
	Structures    map[int]EnemyDefenseIntel
	Capturables   map[int]EnemyDefenseIntel
	UnitsSeen     map[string]int
	BuildingsSeen map[string]int
}

func (e *Engine) IntelSnapshot() IntelSnapshot {
	e.memMu.Lock()
	defer e.memMu.Unlock()
	return IntelSnapshot{Bases: maps.Clone(getEnemyBases(e.Memory)), Structures: maps.Clone(getEnemyStructures(e.Memory)), Capturables: maps.Clone(getCapturables(e.Memory)), UnitsSeen: maps.Clone(GetEnemyUnitsSeen(e.Memory)), BuildingsSeen: maps.Clone(GetEnemyBuildingsSeen(e.Memory))}
}

func (e *Engine) ThreatSnapshot(gs model.GameState) *model.ThreatField {
	e.memMu.Lock()
	defer e.memMu.Unlock()
	e.mu.RLock()
	terrain := e.Terrain
	e.mu.RUnlock()
	field := ThreatFieldFor(e.Memory, terrain, gs)
	if field == nil {
		return nil
	}
	copy := *field
	copy.Cells = slices.Clone(field.Cells)
	return &copy
}
