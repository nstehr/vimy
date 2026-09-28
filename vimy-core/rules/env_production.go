package rules

import (
	"math"
	"slices"
	"strings"
)

// CanBuildAnyCombatVehicle reports whether the generic production rule has
// anything worth building.
//
// Roles with their own capped rules are excluded, so when a flak truck is the
// only option produce-vehicle holds instead of building one. Holding is right:
// the dedicated rule still fills its cap, and the cash stays free for the radar
// that unlocks a real combat vehicle.
func (e RuleEnv) CanBuildAnyCombatVehicle() bool {
	for _, r := range genericVehicleRoles(combatVehicleRoles) {
		if e.CanBuildRole(r) {
			return true
		}
	}
	return false
}

func (e RuleEnv) CombatVehicleCount() int {
	n := 0
	for _, r := range combatVehicleRoles {
		n += e.RoleCount(r)
	}
	return n
}

// BestBuildableVehicle checks LLM preferences before combatVehicleRoles.
func (e RuleEnv) BestBuildableVehicle() string {
	// The preference list gets the same capped-role filter as the fallback:
	// otherwise a doctrine listing flak_truck behind two unbuildable tanks
	// lands on it every time and the fallback is never reached.
	if item := e.bestBuildableFrom(genericVehicleRoles(e.Preferences.Vehicle), nil); item != "" {
		return item
	}
	return e.bestBuildableFrom(genericVehicleRoles(combatVehicleRoles), nil)
}

func (e RuleEnv) CanBuildAnyCombatAircraft() bool {
	for _, r := range combatAircraftRoles {
		if e.CanBuildRole(r) {
			return true
		}
	}
	return false
}

func (e RuleEnv) CombatAircraftCount() int {
	n := 0
	for _, r := range combatAircraftRoles {
		n += e.RoleCount(r)
	}
	return n
}

func (e RuleEnv) CanBuildAnySpecialist() bool {
	for _, r := range specialistInfantryRoles {
		if e.CanBuildRole(r) {
			return true
		}
	}
	return false
}

func (e RuleEnv) SpecialistInfantryCount() int {
	n := 0
	for _, r := range specialistInfantryRoles {
		n += e.RoleCount(r)
	}
	return n
}

// BestBuildableSpecialist picks elite infantry, preferences first. Preferences
// are filtered to specialists so an engineer or rocket soldier can't hijack the
// slot.
func (e RuleEnv) BestBuildableSpecialist() string {
	specialistSet := make(map[string]bool, len(specialistInfantryRoles))
	for _, r := range specialistInfantryRoles {
		specialistSet[r] = true
	}
	if item := e.bestBuildableFrom(e.Preferences.Infantry, specialistSet); item != "" {
		return item
	}
	return e.bestBuildableFrom(specialistInfantryRoles, nil)
}

// bestBuildableFrom picks the buildable candidate with the fewest existing
// units, ties going to list order. Production cycles across roles without
// losing preference. A non-nil allowSet restricts which roles are eligible.
func (e RuleEnv) bestBuildableFrom(candidates []string, allowSet map[string]bool) string {
	minCount := math.MaxInt
	for _, r := range candidates {
		if allowSet != nil && !allowSet[r] {
			continue
		}
		if e.BuildableType(r) == "" {
			continue
		}
		if c := e.RoleCount(r); c < minCount {
			minCount = c
		}
	}
	if minCount == math.MaxInt {
		return "" // nothing buildable
	}
	for _, r := range candidates {
		if allowSet != nil && !allowSet[r] {
			continue
		}
		if e.RoleCount(r) != minCount {
			continue
		}
		if item := e.BuildableType(r); item != "" {
			return item
		}
	}
	return ""
}

// BestBuildableAircraft checks LLM preferences before combatAircraftRoles.
func (e RuleEnv) BestBuildableAircraft() string {
	if item := e.bestBuildableFrom(e.Preferences.Aircraft, nil); item != "" {
		return item
	}
	return e.bestBuildableFrom(combatAircraftRoles, nil)
}

// BestBuildableNaval checks LLM preferences before combatNavalRoles.
func (e RuleEnv) BestBuildableNaval() string {
	if item := e.bestBuildableFrom(e.Preferences.Naval, nil); item != "" {
		return item
	}
	return e.bestBuildableFrom(combatNavalRoles, nil)
}

// HasRole abstracts over faction-specific types (e.g. "barracks" matches both "barr" and "tent").
func (e RuleEnv) HasRole(name string) bool {
	r, ok := roles[name]
	if !ok {
		return false
	}
	return containsAnyType(e.State.Buildings, r.types) || containsAnyType(e.State.Units, r.types)
}

func (e RuleEnv) RoleCount(name string) int {
	r, ok := roles[name]
	if !ok {
		return 0
	}
	return countAnyType(e.State.Buildings, r.types) + countAnyType(e.State.Units, r.types)
}

func (e RuleEnv) CanBuildRole(name string) bool {
	r, ok := roles[name]
	if !ok {
		return false
	}
	for _, pq := range e.State.ProductionQueues {
		if strings.EqualFold(pq.Type, r.queue) {
			for _, t := range r.types {
				if slices.ContainsFunc(pq.Buildable, func(s string) bool {
					return matchesType(s, t)
				}) {
					return true
				}
			}
			return false
		}
	}
	return false
}

// BuildableType resolves a role to its actual buildable name for the current faction
// (e.g. "barracks" → "tent" for Allies, "barr" for Soviets).
func (e RuleEnv) BuildableType(name string) string {
	r, ok := roles[name]
	if !ok {
		return ""
	}
	for _, pq := range e.State.ProductionQueues {
		if strings.EqualFold(pq.Type, r.queue) {
			for _, t := range r.types {
				idx := slices.IndexFunc(pq.Buildable, func(s string) bool {
					return matchesType(s, t)
				})
				if idx >= 0 {
					return pq.Buildable[idx]
				}
			}
			return ""
		}
	}
	return ""
}
