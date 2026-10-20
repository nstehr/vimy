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
	// The preference list is an ORDER, so the first buildable entry wins; the
	// fallback below is an enumeration, so it keeps least-built cycling. A
	// preference that cannot be paid for does not block a cheaper one behind
	// it -- preferredFrom runs the affordable pass before the buildable one --
	// which is what the capped-role filter was guarding against here.
	if item := e.preferredFrom(genericVehicleRoles(e.Preferences.Vehicle), nil); item != "" {
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
	if item := e.preferredFrom(e.Preferences.Infantry, specialistSet); item != "" {
		return item
	}
	return e.bestBuildableFrom(specialistInfantryRoles, nil)
}

// bestBuildableFrom picks the buildable candidate with the fewest existing
// units, ties going to list order. Production cycles across roles without
// losing preference. A non-nil allowSet restricts which roles are eligible.
// bestBuildableFrom prefers what can be paid for, and only then what can merely
// be built.
//
// Two passes rather than one, because the least-built role wins and a role never
// built has count zero. As SOVIET that is heavy_tank, which resolves to a
// 2000-credit mammoth, so it won every contest and the order was re-sent every
// 100 ticks without ever completing: game 192 issued 169 tank orders across this
// rule and produce-heavy-vehicle and built ZERO tanks. The second pass keeps the
// old behaviour when nothing at all is affordable, so this never builds less
// than before -- an order placed with no cash still progresses as cash arrives,
// which is the right thing when there is no cheaper option.
func (e RuleEnv) bestBuildableFrom(candidates []string, allowSet map[string]bool) string {
	if item := e.bestFrom(candidates, allowSet, e.AffordableType); item != "" {
		return item
	}
	return e.bestFrom(candidates, allowSet, e.BuildableType)
}

// preferredFrom honours a preference list as a WEIGHTED order: every entry gets
// built, the head gets built most, and the ratio follows list position.
//
// Position i of n carries weight n-i, so [artillery, medium_tank, light_tank]
// weighs 3:2:1. The pick is the candidate whose count is furthest below its
// share, argmin of RoleCount(r)/weight(r), ties to the earlier position. Repeated
// picks converge on the weights.
//
// This sits between two things that were both wrong. LEAST-BUILT cycling, which
// bestBuildableFrom still does for the fallback enumerations, ignores position
// entirely: the strategist asked for [artillery, medium_tank, light_tank] in 223
// of 233 doctrine windows across games 217-224 and got the THIRD choice as often
// as the first and the SECOND least of all -- game 219 built 20/8/24, game 220
// 11/3/11, game 222 10/6/10 -- which is the one-or-two-of-each army. STRICT
// first-buildable order, tried on 2026-10-11, overcorrected: artillery and
// medium_tank both cost 850, so the head won whenever cash cleared 850 and the
// equally-priced second entry became unreachable. Game 225 built 7 artillery, 0
// medium, 6 light, and its artillery:light ratio of 1.17 was IDENTICAL to game
// 224's before the change -- the only realised effect was deleting a unit class,
// with d/k 1.59 and 1 building killed, both inside the cohort band. No upside,
// one less arm.
//
// Weighting keeps every entry in the army while making the order mean something,
// which is what a ranked list from the strategist plainly intends.
//
// Affordable before merely buildable, as bestBuildableFrom does: a never-built
// role has count zero and resolves to whatever is dearest, which is how game 192
// issued 169 tank orders and built no tanks. The weighting applies within each
// pass, so an unaffordable head does not block a cheaper entry behind it.
func (e RuleEnv) preferredFrom(candidates []string, allowSet map[string]bool) string {
	n := len(candidates)
	for _, resolve := range []func(string) string{e.AffordableType, e.BuildableType} {
		best, bestScore := "", math.Inf(1)
		for i, r := range candidates {
			if allowSet != nil && !allowSet[r] {
				continue
			}
			item := resolve(r)
			if item == "" {
				continue
			}
			// weight n-i: first position heaviest, last position 1.
			score := float64(e.RoleCount(r)) / float64(n-i)
			if score < bestScore {
				best, bestScore = item, score
			}
		}
		if best != "" {
			return best
		}
	}
	return ""
}

func (e RuleEnv) bestFrom(candidates []string, allowSet map[string]bool, resolve func(string) string) string {
	minCount := math.MaxInt
	for _, r := range candidates {
		if allowSet != nil && !allowSet[r] {
			continue
		}
		if resolve(r) == "" {
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
		if item := resolve(r); item != "" {
			return item
		}
	}
	return ""
}

// BestBuildableAircraft checks LLM preferences before combatAircraftRoles.
func (e RuleEnv) BestBuildableAircraft() string {
	if item := e.preferredFrom(e.Preferences.Aircraft, nil); item != "" {
		return item
	}
	return e.bestBuildableFrom(combatAircraftRoles, nil)
}

// BestBuildableNaval checks LLM preferences before combatNavalRoles.
func (e RuleEnv) BestBuildableNaval() string {
	if item := e.preferredFrom(e.Preferences.Naval, nil); item != "" {
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
// AffordableType is the first type of a role that is buildable AND costs no
// more than the cash in hand, in the role's own order.
//
// BuildableType returns the heaviest buildable type, and the engine's
// BuildableItems() only filters on price when PayUpFront is set, which
// ClassicProductionQueue does not. So as SOVIET the heavy_tank role resolves to
// a 2000-credit mammoth that game 192 could afford in 15 of 699 sampled states,
// the order went out, could not be paid, and the envelope was re-sent every 100
// ticks: 169 tank orders across produce-vehicle and produce-heavy-vehicle, and
// ZERO tanks built, while the 1150 heavy tank behind it in the list -- affordable
// in 138 of those states -- was never reached. As ALLIED the mammoth is never
// buildable so it fell straight through to a 2tnk at 850, which is why the bug
// was invisible until Vimy played Soviet.
//
// Empty when nothing in the role is affordable, so a caller can fall through to
// the next role and then to BuildableType, never doing less than before.
func (e RuleEnv) AffordableType(name string) string {
	r, ok := roles[name]
	if !ok {
		return ""
	}
	cash := e.Cash()
	for _, pq := range e.State.ProductionQueues {
		if !strings.EqualFold(pq.Type, r.queue) {
			continue
		}
		for _, t := range r.types {
			for _, b := range pq.Buildable {
				if !matchesType(b, t) {
					continue
				}
				// No recorded cost means an older mod build that does not send
				// them; treat it as affordable rather than refusing to build.
				if cost, known := pq.BuildableCosts[b]; !known || cost <= cash {
					return b
				}
			}
		}
		return ""
	}
	return ""
}

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
