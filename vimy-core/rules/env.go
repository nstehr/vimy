package rules

import (
	"math"
	"slices"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
)

// UnitPreferences are the LLM's per-category role rankings, consulted by the
// BestBuildable* functions before the hardcoded fallback order.
type UnitPreferences struct {
	Infantry []string
	Vehicle  []string
	Aircraft []string
	Naval    []string
}

// RuleEnv is the expression evaluation context. All exported methods are
// callable from expr rule conditions (e.g. `Cash() >= 500`).
type RuleEnv struct {
	State       model.GameState
	Faction     string
	Memory      map[string]any
	Signals     StrategicSignals
	Policy      DoctrinePolicy
	result      *ActionResult
	Terrain     *model.TerrainGrid
	Preferences UnitPreferences
	TargetBias  TargetBias
	// Events is where structured telemetry goes. Nil when streaming is off,
	// which is the default and the case every test runs in.
	Events TelemetrySink
}

func biasOr1(b float64) float64 {
	if b == 0 {
		return 1.0
	}
	return b
}

func (e RuleEnv) HasUnit(t string) bool { return containsType(e.State.Units, t) }

func (e RuleEnv) HasBuilding(t string) bool { return containsType(e.State.Buildings, t) }

func (e RuleEnv) UnitCount(t string) int { return countType(e.State.Units, t) }

func (e RuleEnv) BuildingCount(t string) int { return countType(e.State.Buildings, t) }

// QueueDepth is how many items a queue is carrying, in progress plus waiting.
//
// Every production rule gated on QueueBusy, so Vimy built strictly one item at
// a time and never queued ahead: between an item completing and a rule noticing,
// the queue sat idle, and in that gap whatever cleared the lowest cash
// threshold took the money. A human commits future income by queuing now — five
// tanks clicked at once drain cash as they build. Depth is what lets a rule do
// the same.
func (e RuleEnv) QueueDepth(q string) int {
	depth := 0
	for _, pq := range e.State.ProductionQueues {
		if !strings.EqualFold(pq.Type, q) {
			continue
		}
		n := len(pq.Items)
		// Items has been seen empty while something was plainly building, so
		// an in-progress item counts for itself when the list does not show it.
		if n == 0 && pq.CurrentItem != "" {
			n = 1
		}
		if n > depth {
			depth = n
		}
	}
	return depth
}

func (e RuleEnv) QueueBusy(q string) bool {
	found := false
	for _, pq := range e.State.ProductionQueues {
		if strings.EqualFold(pq.Type, q) {
			found = true
			if pq.CurrentItem == "" || pq.CurrentProgress >= 100 {
				return false // at least one queue is free
			}
		}
	}
	return found // true only if all matched queues are busy (or none found)
}

func (e RuleEnv) QueueReady(q string) bool {
	for _, pq := range e.State.ProductionQueues {
		if strings.EqualFold(pq.Type, q) {
			if pq.CurrentItem != "" && pq.CurrentProgress >= 100 {
				return true
			}
		}
	}
	return false
}

// QueueProducingRole covers the cases QueueBusy misses — an item sitting at
// 100%, or a second free queue — where the role is nonetheless in production.
func (e RuleEnv) QueueProducingRole(name string) bool {
	r, ok := roles[name]
	if !ok {
		return false
	}
	for _, pq := range e.State.ProductionQueues {
		if !strings.EqualFold(pq.Type, r.queue) {
			continue
		}
		for _, t := range r.types {
			if matchesType(pq.CurrentItem, t) {
				return true
			}
		}
		for _, item := range pq.Items {
			for _, t := range r.types {
				if matchesType(item, t) {
					return true
				}
			}
		}
	}
	return false
}

func (e RuleEnv) CanBuild(q, item string) bool {
	for _, pq := range e.State.ProductionQueues {
		if strings.EqualFold(pq.Type, q) {
			return slices.ContainsFunc(pq.Buildable, func(s string) bool {
				return matchesType(s, item)
			})
		}
	}
	return false
}

func (e RuleEnv) Cash() int {
	return e.State.Player.Cash + e.State.Player.Resources
}

// IncomeRate is net cash change over the last sampling window, in credits.
//
// Net, not gross: it goes to zero exactly when every credit is committed as it
// arrives — the pathology the savings model exists to answer, and one a gross
// figure reports as a healthy economy.
//
// Sampled by the engine: a rule environment sees one tick, and a rate needs two.
func (e RuleEnv) IncomeRate() int {
	v, _ := e.Memory["incomeRate"].(int)
	return v
}

func (e RuleEnv) PowerExcess() int {
	return e.State.Player.PowerProvided - e.State.Player.PowerDrained
}

func (e RuleEnv) IdleHarvesters() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && matchesType(u.Type, Harvester) {
			out = append(out, u)
		}
	}
	return out
}

func (e RuleEnv) NearestEnemy() *model.Enemy {
	if len(e.State.Enemies) == 0 {
		return nil
	}
	bx, by := 0, 0
	if ax, ay, ok := e.baseAnchor(); ok {
		bx = ax
		by = ay
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

// recordSuperweaponFire tracks launches so the strategist LLM can see fire history.
func recordSuperweaponFire(env RuleEnv, key string) {
	fires, _ := env.Memory["superweaponFires"].(map[string]int)
	if fires == nil {
		fires = make(map[string]int)
	}
	fires[key]++
	env.Memory["superweaponFires"] = fires
}

// GetSuperweaponFires returns cumulative fire counts (used by strategist summarizer).
func GetSuperweaponFires(memory map[string]any) map[string]int {
	if v, ok := memory["superweaponFires"].(map[string]int); ok {
		return v
	}
	return nil
}

// rebuildableRoles get rebuild rules when destroyed; membership is what makes
// LostRole track them.
var rebuildableRoles = []string{
	"power_plant", "advanced_power",
	"barracks", "war_factory", "radar", "tech_center", "airfield", "naval_yard", "refinery", "service_depot",
	"missile_silo", "iron_curtain",
}

// updateBuiltRoles is the record LostRole diffs against — game state alone
// can't distinguish "never built" from "destroyed".
func updateBuiltRoles(env RuleEnv) {
	builtRoles, _ := env.Memory["builtRoles"].(map[string]bool)
	if builtRoles == nil {
		builtRoles = make(map[string]bool)
	}
	for _, name := range rebuildableRoles {
		if env.HasRole(name) {
			builtRoles[name] = true
		}
	}
	env.Memory["builtRoles"] = builtRoles
}

// LostRole detects destruction: true if we had this building before but don't now.
func (e RuleEnv) LostRole(name string) bool {
	builtRoles, _ := e.Memory["builtRoles"].(map[string]bool)
	return builtRoles[name] && !e.HasRole(name)
}
