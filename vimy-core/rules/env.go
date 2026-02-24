package rules

import (
	"math"
	"slices"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
)

// RuleEnv is the expression evaluation context. All exported methods are
// callable from expr rule conditions (e.g. `Cash() >= 500`).
type RuleEnv struct {
	State   model.GameState
	Faction string
	Memory  map[string]any
	Terrain *model.TerrainGrid
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

// TerrainAt converts map coordinates to coarse grid and returns the terrain type.
// Returns Land if no terrain grid is available (safe default).
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

// MapHasWater returns true if any zone in the terrain grid is water.
// Returns false if no terrain grid is available (don't gate naval on missing data).
func (e RuleEnv) MapHasWater() bool {
	if e.Terrain == nil {
		return true // assume water possible when no terrain data
	}
	return e.Terrain.HasWater()
}

func (e RuleEnv) EnemiesVisible() bool { return len(e.State.Enemies) > 0 }

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

// IdleGroundUnits returns idle land combat units — excludes economic units
// (harvesters, MCVs) and other domains (aircraft, naval).
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

func (e RuleEnv) CapturableCount() int { return len(e.State.Capturables) }

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

func (e RuleEnv) IdleEngineers() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && matchesType(u.Type, Engineer) {
			out = append(out, u)
		}
	}
	return out
}

func (e RuleEnv) SupportPowerReady(key string) bool {
	for _, sp := range e.State.SupportPowers {
		if strings.EqualFold(sp.Key, key) {
			return sp.Ready
		}
	}
	return false
}

func (e RuleEnv) HasSupportPower(key string) bool {
	for _, sp := range e.State.SupportPowers {
		if strings.EqualFold(sp.Key, key) {
			return true
		}
	}
	return false
}

// GroundUnitCentroid returns where idle ground units are clustered.
// Used to target iron curtain on our own forces.
func (e RuleEnv) GroundUnitCentroid() (int, int) {
	idle := e.IdleGroundUnits()
	if len(idle) > 0 {
		sumX, sumY := 0, 0
		for _, u := range idle {
			sumX += u.X
			sumY += u.Y
		}
		return sumX / len(idle), sumY / len(idle)
	}
	if len(e.State.Buildings) > 0 {
		sumX, sumY := 0, 0
		for _, b := range e.State.Buildings {
			sumX += b.X
			sumY += b.Y
		}
		return sumX / len(e.State.Buildings), sumY / len(e.State.Buildings)
	}
	return 0, 0
}

// ResourcesNearCap triggers ore silo construction before resources overflow.
func (e RuleEnv) ResourcesNearCap() bool {
	if e.State.Player.ResourceCapacity <= 0 {
		return false
	}
	return float64(e.State.Player.Resources) > 0.8*float64(e.State.Player.ResourceCapacity)
}

func (e RuleEnv) SquadExists(name string) bool {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	return ok && len(sq.UnitIDs) > 0
}

func (e RuleEnv) SquadSize(name string) int {
	squads := getSquads(e.Memory)
	if sq, ok := squads[name]; ok {
		return len(sq.UnitIDs)
	}
	return 0
}

func (e RuleEnv) SquadIdleCount(name string) int {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok {
		return 0
	}
	idleSet := make(map[int]bool)
	for _, u := range e.State.Units {
		if u.Idle {
			idleSet[u.ID] = true
		}
	}
	n := 0
	for _, id := range sq.UnitIDs {
		if idleSet[id] {
			n++
		}
	}
	return n
}

func (e RuleEnv) UnassignedIdleGround() []model.Unit {
	assigned := squadUnitIDSet(e.Memory)
	var out []model.Unit
	for _, u := range e.IdleGroundUnits() {
		if !assigned[u.ID] {
			out = append(out, u)
		}
	}
	return out
}

func (e RuleEnv) UnassignedIdleAir() []model.Unit {
	assigned := squadUnitIDSet(e.Memory)
	var out []model.Unit
	for _, u := range e.IdleCombatAircraft() {
		if !assigned[u.ID] {
			out = append(out, u)
		}
	}
	return out
}

func (e RuleEnv) UnassignedIdleNaval() []model.Unit {
	assigned := squadUnitIDSet(e.Memory)
	var out []model.Unit
	for _, u := range e.IdleNavalUnits() {
		if !assigned[u.ID] {
			out = append(out, u)
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

// rebuildableRoles lists roles that get rebuild rules if destroyed. Tracked in
// memory so LostRole() can detect when a previously-owned building is gone.
var rebuildableRoles = []string{
	"power_plant", "advanced_power",
	"barracks", "war_factory", "radar", "tech_center", "airfield", "naval_yard", "refinery", "service_depot",
	"missile_silo", "iron_curtain",
}

// updateBuiltRoles records which roles exist so LostRole() can later detect
// destruction. Without this, the AI wouldn't know to rebuild something
// it once had.
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

// EnemyBaseIntel records a known enemy base position.
type EnemyBaseIntel struct {
	Owner         string
	X             int
	Y             int
	Tick          int
	FromBuildings bool // true if derived from building sightings (high confidence)
}

// knownBuildingTypes distinguishes enemy buildings from mobile units in the
// Enemies list. Building sightings give high-confidence base positions;
// unit sightings might just be an attack force passing through.
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

// isKnownBuildingType handles faction variants (e.g. "afld.ukraine" → "afld").
func isKnownBuildingType(t string) bool {
	base := strings.ToLower(t)
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	return knownBuildingTypes[base]
}

// updateIntel maintains a map of known enemy base positions. Building sightings
// always update (high confidence); unit sightings only seed initial intel to
// avoid overwriting a known base location with a roaming attack force.
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

	// Building sightings always overwrite — structures don't move.
	for owner, a := range buildingsByOwner {
		bases[owner] = EnemyBaseIntel{
			Owner:         owner,
			X:             a.sumX / a.count,
			Y:             a.sumY / a.count,
			Tick:          env.State.Tick,
			FromBuildings: true,
		}
	}

	// Unit sightings only seed initial intel — don't let a passing enemy
	// patrol overwrite a confirmed building-based position.
	for owner, a := range unitsByOwner {
		if _, exists := bases[owner]; exists {
			continue
		}
		bases[owner] = EnemyBaseIntel{
			Owner:         owner,
			X:             a.sumX / a.count,
			Y:             a.sumY / a.count,
			Tick:          env.State.Tick,
			FromBuildings: false,
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

// HasEnemyIntel requires building-based intel. Unit-only sightings don't
// count — scouting should continue until we find the actual base.
func (e RuleEnv) HasEnemyIntel() bool {
	for _, base := range getEnemyBases(e.Memory) {
		if base.FromBuildings {
			return true
		}
	}
	return false
}

// NearestEnemyBase returns the closest remembered enemy base for fog-of-war attacks.
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

func (e RuleEnv) EnemyBaseCount() int {
	return len(getEnemyBases(e.Memory))
}

// BaseUnderAttack uses a 20% map-diagonal proximity threshold. This avoids
// false positives from distant enemies while catching attacks that haven't
// reached buildings yet.
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

func (e RuleEnv) CanBuildAnyCombatVehicle() bool {
	for _, r := range combatVehicleRoles {
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

// BestBuildableVehicle returns the highest-priority buildable combat vehicle.
// Priority order comes from combatVehicleRoles (mammoth > heavy > light > ...).
func (e RuleEnv) BestBuildableVehicle() string {
	for _, r := range combatVehicleRoles {
		if item := e.BuildableType(r); item != "" {
			return item
		}
	}
	return ""
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

// BestBuildableSpecialist picks the best available elite infantry
// (priority: tanya > shock trooper > flamethrower > medic).
func (e RuleEnv) BestBuildableSpecialist() string {
	for _, r := range specialistInfantryRoles {
		if item := e.BuildableType(r); item != "" {
			return item
		}
	}
	return ""
}

// BestBuildableAircraft picks the best available combat aircraft
// (prefers advanced: longbow/MiG over basic: blackhawk/yak).
func (e RuleEnv) BestBuildableAircraft() string {
	for _, r := range combatAircraftRoles {
		if item := e.BuildableType(r); item != "" {
			return item
		}
	}
	return ""
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
