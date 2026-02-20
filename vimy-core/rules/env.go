package rules

import (
	"math"
	"slices"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
)

// RuleEnv wraps game state and exposes helper methods callable from expr expressions.
type RuleEnv struct {
	State  model.GameState
	Memory map[string]any
}

func (e RuleEnv) HasUnit(t string) bool {
	for _, u := range e.State.Units {
		if strings.EqualFold(u.Type, t) {
			return true
		}
	}
	return false
}

func (e RuleEnv) HasBuilding(t string) bool {
	for _, b := range e.State.Buildings {
		if strings.EqualFold(b.Type, t) {
			return true
		}
	}
	return false
}

func (e RuleEnv) UnitCount(t string) int {
	n := 0
	for _, u := range e.State.Units {
		if strings.EqualFold(u.Type, t) {
			n++
		}
	}
	return n
}

func (e RuleEnv) BuildingCount(t string) int {
	n := 0
	for _, b := range e.State.Buildings {
		if strings.EqualFold(b.Type, t) {
			n++
		}
	}
	return n
}

func (e RuleEnv) QueueBusy(q string) bool {
	for _, pq := range e.State.ProductionQueues {
		if strings.EqualFold(pq.Type, q) {
			return pq.CurrentItem != "" && pq.CurrentProgress < 100
		}
	}
	return false
}

func (e RuleEnv) QueueReady(q string) bool {
	for _, pq := range e.State.ProductionQueues {
		if strings.EqualFold(pq.Type, q) {
			return pq.CurrentItem != "" && pq.CurrentProgress >= 100
		}
	}
	return false
}

func (e RuleEnv) CanBuild(q, item string) bool {
	for _, pq := range e.State.ProductionQueues {
		if strings.EqualFold(pq.Type, q) {
			return slices.ContainsFunc(pq.Buildable, func(s string) bool {
				return strings.EqualFold(s, item)
			})
		}
	}
	return false
}

func (e RuleEnv) Cash() int {
	return e.State.Player.Cash
}

func (e RuleEnv) PowerExcess() int {
	return e.State.Player.PowerProvided - e.State.Player.PowerDrained
}

func (e RuleEnv) IdleMilitaryUnits() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && !strings.EqualFold(u.Type, "harv") {
			out = append(out, u)
		}
	}
	return out
}

func (e RuleEnv) IdleHarvesters() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && strings.EqualFold(u.Type, "harv") {
			out = append(out, u)
		}
	}
	return out
}

func (e RuleEnv) NearestEnemy() *model.Enemy {
	if len(e.State.Enemies) == 0 {
		return nil
	}
	// Use first building as base reference, or (0,0) if none.
	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}
	var nearest *model.Enemy
	bestDist := math.MaxFloat64
	for i := range e.State.Enemies {
		dx := float64(e.State.Enemies[i].X - bx)
		dy := float64(e.State.Enemies[i].Y - by)
		d := math.Sqrt(dx*dx + dy*dy)
		if d < bestDist {
			bestDist = d
			nearest = &e.State.Enemies[i]
		}
	}
	return nearest
}

func (e RuleEnv) DamagedBuildings() []model.Building {
	var out []model.Building
	for _, b := range e.State.Buildings {
		if b.MaxHP > 0 && float64(b.HP)/float64(b.MaxHP) < 0.75 {
			out = append(out, b)
		}
	}
	return out
}

func (e RuleEnv) MapWidth() int  { return e.State.MapWidth }
func (e RuleEnv) MapHeight() int { return e.State.MapHeight }

func (e RuleEnv) EnemiesVisible() bool { return len(e.State.Enemies) > 0 }

// HasBarracks returns true if the player has a barracks (Allied "tent" or Soviet "barr").
func (e RuleEnv) HasBarracks() bool {
	return e.HasBuilding("tent") || e.HasBuilding("barr")
}

// CanBuildBarracks returns true if a barracks is available in the Building queue's buildable list.
func (e RuleEnv) CanBuildBarracks() bool {
	return e.CanBuild("Building", "tent") || e.CanBuild("Building", "barr")
}
