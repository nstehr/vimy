package rules

import (
	"math"
	"slices"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
)

// RuleEnv wraps game state and exposes helper methods callable from expr expressions.
type RuleEnv struct {
	State   model.GameState
	Faction string
	Memory  map[string]any
}

func (e RuleEnv) HasUnit(t string) bool      { return containsType(e.State.Units, t) }
func (e RuleEnv) HasBuilding(t string) bool   { return containsType(e.State.Buildings, t) }
func (e RuleEnv) UnitCount(t string) int      { return countType(e.State.Units, t) }
func (e RuleEnv) BuildingCount(t string) int  { return countType(e.State.Buildings, t) }

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
				return matchesType(s, item)
			})
		}
	}
	return false
}

func (e RuleEnv) Cash() int {
	return e.State.Player.Cash + e.State.Player.Resources
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

// isAircraft returns true if the unit type matches any combat aircraft role.
func isAircraft(u model.Unit) bool {
	for _, r := range combatAircraftRoles {
		role := roles[r]
		for _, t := range role.types {
			if matchesType(u.Type, t) {
				return true
			}
		}
	}
	return false
}

// isNaval returns true if the unit type matches any combat naval role.
func isNaval(u model.Unit) bool {
	for _, r := range combatNavalRoles {
		role := roles[r]
		for _, t := range role.types {
			if matchesType(u.Type, t) {
				return true
			}
		}
	}
	return false
}

// IdleGroundUnits returns idle units excluding harvesters, MCVs, aircraft, and naval units.
func (e RuleEnv) IdleGroundUnits() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if !u.Idle {
			continue
		}
		if matchesType(u.Type, Harvester) || matchesType(u.Type, MCV) {
			continue
		}
		if isAircraft(u) || isNaval(u) {
			continue
		}
		out = append(out, u)
	}
	return out
}

// IdleNavalUnits returns idle units matching any combat naval role.
func (e RuleEnv) IdleNavalUnits() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if !u.Idle {
			continue
		}
		if isNaval(u) {
			out = append(out, u)
		}
	}
	return out
}

// IdleCombatAircraft returns idle units matching any combat aircraft role.
func (e RuleEnv) IdleCombatAircraft() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if !u.Idle {
			continue
		}
		for _, r := range combatAircraftRoles {
			role := roles[r]
			for _, t := range role.types {
				if matchesType(u.Type, t) {
					out = append(out, u)
					goto next
				}
			}
		}
	next:
	}
	return out
}

// CapturableCount returns the number of visible capturable buildings.
func (e RuleEnv) CapturableCount() int { return len(e.State.Capturables) }

// NearestCapturable returns the closest capturable building to our base.
func (e RuleEnv) NearestCapturable() *model.Enemy {
	if len(e.State.Capturables) == 0 {
		return nil
	}
	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}
	var nearest *model.Enemy
	bestDist := math.MaxFloat64
	for i := range e.State.Capturables {
		dx := float64(e.State.Capturables[i].X - bx)
		dy := float64(e.State.Capturables[i].Y - by)
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			nearest = &e.State.Capturables[i]
		}
	}
	return nearest
}

// IdleEngineers returns idle units matching the engineer type.
func (e RuleEnv) IdleEngineers() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && matchesType(u.Type, Engineer) {
			out = append(out, u)
		}
	}
	return out
}

// ResourcesNearCap returns true when stored resources exceed 80% of capacity.
func (e RuleEnv) ResourcesNearCap() bool {
	if e.State.Player.ResourceCapacity <= 0 {
		return false
	}
	return float64(e.State.Player.Resources) > 0.8*float64(e.State.Player.ResourceCapacity)
}

// rebuildableRoles is the fixed list of roles eligible for rebuild tracking.
var rebuildableRoles = []string{
	"barracks", "war_factory", "radar", "tech_center", "airfield", "naval_yard", "refinery", "service_depot",
}

// updateBuiltRoles tracks which roles the AI has successfully built.
// Called at the start of each evaluation to maintain memory of past builds.
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

// LostRole returns true if the role was previously built but no longer exists.
func (e RuleEnv) LostRole(name string) bool {
	builtRoles, _ := e.Memory["builtRoles"].(map[string]bool)
	return builtRoles[name] && !e.HasRole(name)
}

// EnemyBaseIntel records a known enemy base position.
type EnemyBaseIntel struct {
	Owner string
	X     int
	Y     int
	Tick  int
}

// knownBuildingTypes is the set of RA building type names used to identify
// enemy structures (as opposed to mobile units).
var knownBuildingTypes = map[string]bool{
	// Production
	"fact": true, "barr": true, "tent": true, "weap": true, "kenn": true,
	"afld": true, "hpad": true, "syrd": true, "spen": true,
	// Economy
	"powr": true, "apwr": true, "proc": true, "silo": true,
	// Tech / Support
	"dome": true, "atek": true, "stek": true, "fix": true,
	// Superweapons
	"iron": true, "pdox": true, "mslo": true, "gap": true,
	// Defenses
	"pbox": true, "hbox": true, "gun": true, "ftur": true,
	"tsla": true, "agun": true, "sam": true,
}

// isKnownBuildingType returns true if the type (possibly a faction variant like "afld.ukraine")
// matches any entry in knownBuildingTypes.
func isKnownBuildingType(t string) bool {
	base := strings.ToLower(t)
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	return knownBuildingTypes[base]
}

// updateIntel records the centroid of visible enemies per owner.
// Prefers building positions (high confidence) but falls back to unit
// positions when no buildings are visible for that owner.
// Called at the start of each evaluation.
func updateIntel(env RuleEnv) {
	type acc struct {
		sumX, sumY, count int
	}
	buildingsByOwner := make(map[string]*acc)
	unitsByOwner := make(map[string]*acc)

	for _, e := range env.State.Enemies {
		if isKnownBuildingType(e.Type) {
			a, ok := buildingsByOwner[e.Owner]
			if !ok {
				a = &acc{}
				buildingsByOwner[e.Owner] = a
			}
			a.sumX += e.X
			a.sumY += e.Y
			a.count++
		} else {
			a, ok := unitsByOwner[e.Owner]
			if !ok {
				a = &acc{}
				unitsByOwner[e.Owner] = a
			}
			a.sumX += e.X
			a.sumY += e.Y
			a.count++
		}
	}

	if len(buildingsByOwner) == 0 && len(unitsByOwner) == 0 {
		return
	}

	bases := getEnemyBases(env.Memory)

	// Building sightings always update intel (high confidence).
	for owner, a := range buildingsByOwner {
		bases[owner] = EnemyBaseIntel{
			Owner: owner,
			X:     a.sumX / a.count,
			Y:     a.sumY / a.count,
			Tick:  env.State.Tick,
		}
	}

	// Unit sightings only update intel if we have no existing intel
	// for that owner (avoid overwriting building-based positions with
	// less accurate unit positions or enemy attack forces near our base).
	for owner, a := range unitsByOwner {
		if _, exists := bases[owner]; exists {
			continue
		}
		bases[owner] = EnemyBaseIntel{
			Owner: owner,
			X:     a.sumX / a.count,
			Y:     a.sumY / a.count,
			Tick:  env.State.Tick,
		}
	}

	env.Memory["enemyBases"] = bases
}

func getEnemyBases(memory map[string]any) map[string]EnemyBaseIntel {
	if v, ok := memory["enemyBases"].(map[string]EnemyBaseIntel); ok {
		return v
	}
	return make(map[string]EnemyBaseIntel)
}

// HasEnemyIntel returns true if at least one enemy base position is known.
func (e RuleEnv) HasEnemyIntel() bool {
	bases := getEnemyBases(e.Memory)
	return len(bases) > 0
}

// NearestEnemyBase returns the position of the closest known enemy base
// relative to our buildings.
func (e RuleEnv) NearestEnemyBase() *EnemyBaseIntel {
	bases := getEnemyBases(e.Memory)
	if len(bases) == 0 {
		return nil
	}

	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}

	var nearest *EnemyBaseIntel
	bestDist := math.MaxFloat64
	for _, base := range bases {
		dx := float64(base.X - bx)
		dy := float64(base.Y - by)
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			b := base
			nearest = &b
		}
	}
	return nearest
}

// EnemyBaseCount returns the number of known enemy bases.
func (e RuleEnv) EnemyBaseCount() int {
	return len(getEnemyBases(e.Memory))
}

// BaseUnderAttack returns true if any visible enemy is within 20% of the map
// diagonal of any owned building.
func (e RuleEnv) BaseUnderAttack() bool {
	if len(e.State.Buildings) == 0 || len(e.State.Enemies) == 0 {
		return false
	}
	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	threshold := math.Sqrt(mw*mw+mh*mh) * 0.20
	threshSq := threshold * threshold

	for i := range e.State.Enemies {
		for j := range e.State.Buildings {
			dx := float64(e.State.Enemies[i].X - e.State.Buildings[j].X)
			dy := float64(e.State.Enemies[i].Y - e.State.Buildings[j].Y)
			if dx*dx+dy*dy < threshSq {
				return true
			}
		}
	}
	return false
}

// CanBuildAnyCombatVehicle returns true if any combat vehicle role is buildable in the Vehicle queue.
func (e RuleEnv) CanBuildAnyCombatVehicle() bool {
	for _, r := range combatVehicleRoles {
		if e.CanBuildRole(r) {
			return true
		}
	}
	return false
}

// CombatVehicleCount returns the total count of all owned combat vehicles.
func (e RuleEnv) CombatVehicleCount() int {
	n := 0
	for _, r := range combatVehicleRoles {
		n += e.RoleCount(r)
	}
	return n
}

// BestBuildableVehicle returns the actual item name of the highest-priority buildable combat vehicle, or "".
func (e RuleEnv) BestBuildableVehicle() string {
	for _, r := range combatVehicleRoles {
		if item := e.BuildableType(r); item != "" {
			return item
		}
	}
	return ""
}

// CanBuildAnyCombatAircraft returns true if any combat aircraft role is buildable.
func (e RuleEnv) CanBuildAnyCombatAircraft() bool {
	for _, r := range combatAircraftRoles {
		if e.CanBuildRole(r) {
			return true
		}
	}
	return false
}

// CombatAircraftCount returns the total count of all owned combat aircraft.
func (e RuleEnv) CombatAircraftCount() int {
	n := 0
	for _, r := range combatAircraftRoles {
		n += e.RoleCount(r)
	}
	return n
}

// CanBuildAnySpecialist returns true if any specialist infantry role is buildable.
func (e RuleEnv) CanBuildAnySpecialist() bool {
	for _, r := range specialistInfantryRoles {
		if e.CanBuildRole(r) {
			return true
		}
	}
	return false
}

// SpecialistInfantryCount returns the total count of all owned specialist infantry.
func (e RuleEnv) SpecialistInfantryCount() int {
	n := 0
	for _, r := range specialistInfantryRoles {
		n += e.RoleCount(r)
	}
	return n
}

// BestBuildableSpecialist returns the actual item name of the highest-priority buildable specialist infantry, or "".
func (e RuleEnv) BestBuildableSpecialist() string {
	for _, r := range specialistInfantryRoles {
		if item := e.BuildableType(r); item != "" {
			return item
		}
	}
	return ""
}

// BestBuildableAircraft returns the actual item name of the highest-priority buildable combat aircraft, or "".
func (e RuleEnv) BestBuildableAircraft() string {
	for _, r := range combatAircraftRoles {
		if item := e.BuildableType(r); item != "" {
			return item
		}
	}
	return ""
}

// HasRole returns true if the player has any building or unit matching the role's type variants.
func (e RuleEnv) HasRole(name string) bool {
	r, ok := roles[name]
	if !ok {
		return false
	}
	return containsAnyType(e.State.Buildings, r.types) || containsAnyType(e.State.Units, r.types)
}

// RoleCount returns the total count of buildings and units matching the role's type variants.
func (e RuleEnv) RoleCount(name string) int {
	r, ok := roles[name]
	if !ok {
		return 0
	}
	return countAnyType(e.State.Buildings, r.types) + countAnyType(e.State.Units, r.types)
}

// CanBuildRole returns true if any of the role's type variants appear in the role's queue's buildable list.
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

// BuildableType returns the first buildable type variant for the role, or "" if none.
// Returns the actual buildable name (which may be a faction variant like "afld.ukraine").
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
