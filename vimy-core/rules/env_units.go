package rules

import (
	"log/slog"
	"math"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
)

// DamagedSquadUnits returns idle squad members below the HP threshold.
func (e RuleEnv) DamagedSquadUnits(hpThreshold float64) []model.Unit {
	squadIDs := squadUnitIDSet(e.Memory)
	var out []model.Unit
	for _, u := range e.State.Units {
		if !u.Idle || u.MaxHP == 0 {
			continue
		}
		if !squadIDs[u.ID] {
			continue
		}
		if float64(u.HP)/float64(u.MaxHP) < hpThreshold {
			out = append(out, u)
		}
	}
	return out
}

// DamagedCombatUnits returns combat units below the HP threshold regardless of
// idle or squad status, skipping utility units and anything already retreating.
func (e RuleEnv) DamagedCombatUnits(hpThreshold float64) []model.Unit {
	retreating := getRetreatingUnits(e.Memory)
	scoutID := getScoutID(e.Memory)
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.MaxHP == 0 {
			continue
		}
		if matchesType(u.Type, Harvester) || matchesType(u.Type, MCV) ||
			matchesType(u.Type, Ranger) || matchesType(u.Type, Engineer) ||
			matchesType(u.Type, APC) || isInfantry(u) {
			continue
		}
		if scoutID != 0 && u.ID == scoutID {
			continue
		}
		if _, isRetreating := retreating[u.ID]; isRetreating {
			continue
		}
		if float64(u.HP)/float64(u.MaxHP) < hpThreshold {
			out = append(out, u)
		}
	}
	return out
}

// getRetreatingUnits maps unit ID to the tick the retreat started, for timeout.
func getRetreatingUnits(memory map[string]any) map[int]int {
	if v, ok := memory["retreatingUnits"].(map[int]int); ok {
		return v
	}
	return nil
}

// HasRetreatingUnits returns true if any units are currently retreating.
func (e RuleEnv) HasRetreatingUnits() bool {
	return len(getRetreatingUnits(e.Memory)) > 0
}

// ServiceDepotPos returns the service depot position (bool = exists).
func (e RuleEnv) ServiceDepotPos() (int, int, bool) {
	for _, b := range e.State.Buildings {
		if matchesType(b.Type, ServiceDepot) {
			return b.X, b.Y, true
		}
	}
	return 0, 0, false
}

// ServiceDepot returns the service depot building, or nil if none exists.
func (e RuleEnv) ServiceDepot() *model.Building {
	for i := range e.State.Buildings {
		if matchesType(e.State.Buildings[i].Type, ServiceDepot) {
			return &e.State.Buildings[i]
		}
	}
	return nil
}

// WarFactory returns the first war factory building, or nil if none exists.
func (e RuleEnv) WarFactory() *model.Building {
	for i := range e.State.Buildings {
		if matchesType(e.State.Buildings[i].Type, WarFactory) {
			return &e.State.Buildings[i]
		}
	}
	return nil
}

// Airfield returns the first airfield or helipad building, or nil if none exists.
func (e RuleEnv) Airfield() *model.Building {
	for i := range e.State.Buildings {
		if matchesType(e.State.Buildings[i].Type, Airfield) || matchesType(e.State.Buildings[i].Type, Helipad) {
			return &e.State.Buildings[i]
		}
	}
	return nil
}

// BuildingCentroid returns the average position of all buildings.
func (e RuleEnv) BuildingCentroid() (int, int) {
	if len(e.State.Buildings) == 0 {
		return 0, 0
	}
	sumX, sumY := 0, 0
	for _, b := range e.State.Buildings {
		sumX += b.X
		sumY += b.Y
	}
	return sumX / len(e.State.Buildings), sumY / len(e.State.Buildings)
}

func isInfantry(u model.Unit) bool {
	infantryTypes := []string{RifleInfantry, RocketSoldier, Engineer, Flamethrower,
		ShockTrooper, Tanya, Medic, Grenadier, AttackDog, Spy}
	for _, t := range infantryTypes {
		if matchesType(u.Type, t) {
			return true
		}
	}
	return false
}

// ServiceDepotOrCentroid falls back to the building centroid, then (0, 0).
func (e RuleEnv) ServiceDepotOrCentroid() (int, int) {
	for _, b := range e.State.Buildings {
		if matchesType(b.Type, ServiceDepot) {
			return b.X, b.Y
		}
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

// AircraftCapacity counts physical pad slots — 4 per airfield, 1 per helipad —
// so production can be gated on them. Aircraft queued beyond capacity finish at
// 100% with nowhere to spawn and just churn the queue.
func (e RuleEnv) AircraftCapacity() int {
	var airfields, helipads int
	for _, b := range e.State.Buildings {
		if matchesType(b.Type, Airfield) {
			airfields++
		} else if matchesType(b.Type, Helipad) {
			helipads++
		}
	}
	return 4*airfields + helipads
}

// IsRushed reports the strategist's early-game rush flag: small base plus
// recent harvester pressure. Production rules drop savings reserves on it —
// without a rule-side gate the prompt's rush response never reaches output.
func (e RuleEnv) IsRushed() bool {
	return e.Signals.BeingRushed
}

// IsHarvesterHarassed reports sustained mid-game harvester pressure, which
// rules use to tell a raid apart from a rush.
func (e RuleEnv) IsHarvesterHarassed() bool {
	return e.Signals.HarvesterHarassed
}

// IdleGroundUnits returns idle land combat units, excluding economic and
// utility units and attack dogs — dogs are bite-only, so an offensive squad
// would send them at vehicles.
func (e RuleEnv) IdleGroundUnits() []model.Unit {
	scoutID := getScoutID(e.Memory)
	var out []model.Unit
	for _, u := range e.State.Units {
		if !u.Idle {
			continue
		}
		if matchesType(u.Type, Harvester) || matchesType(u.Type, MCV) || matchesType(u.Type, Ranger) || matchesType(u.Type, Engineer) || matchesType(u.Type, APC) || matchesType(u.Type, Minelayer) || matchesType(u.Type, AttackDog) {
			continue
		}
		if scoutID != 0 && u.ID == scoutID {
			continue // designated scout — handled by scouting rules
		}
		if isAircraft(u) || isNaval(u) {
			continue
		}
		out = append(out, u)
	}
	return out
}

// NearBaseGroundUnits ignores idle status so emergency defense can recall units
// already en route elsewhere. Same 20% map-diagonal threshold as BaseUnderAttack.
func (e RuleEnv) NearBaseGroundUnits() []model.Unit {
	if len(e.State.Buildings) == 0 {
		return nil
	}
	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	threshold := math.Sqrt(mw*mw+mh*mh) * 0.20
	threshSq := threshold * threshold

	var out []model.Unit
	for _, u := range e.State.Units {
		if matchesType(u.Type, Harvester) || matchesType(u.Type, MCV) || matchesType(u.Type, Engineer) || matchesType(u.Type, APC) || matchesType(u.Type, AttackDog) {
			continue
		}
		if isAircraft(u) || isNaval(u) {
			continue
		}
		for j := range e.State.Buildings {
			dx := float64(u.X - e.State.Buildings[j].X)
			dy := float64(u.Y - e.State.Buildings[j].Y)
			if dx*dx+dy*dy < threshSq {
				out = append(out, u)
				break
			}
		}
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

func (e RuleEnv) IdleAPCs() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && matchesType(u.Type, APC) {
			out = append(out, u)
		}
	}
	return out
}

func isTransport(u model.Unit) bool {
	return matchesType(u.Type, APC) || matchesType(u.Type, Ranger)
}

func (e RuleEnv) IdleLoadedAPCs() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && isTransport(u) && u.CargoCount > 0 {
			out = append(out, u)
		}
	}
	return out
}

// APC cargo intent. Game state reports passenger count but not type, so we tag
// at load time and the capture and assault deliver rules each pick their own.
const (
	apcIntentEngineer = "engineer"
	apcIntentCombat   = "combat"
)

// GetAPCCargoIntent returns the intent map, allocating if needed.
func GetAPCCargoIntent(memory map[string]any) map[int]string {
	return memoryMap[int, string](memory, "apcCargoIntent")
}

// pruneAPCCargoIntent drops entries for dead APCs only, never on CargoCount:
// a freshly-tagged APC reads as empty for a tick or two while the server
// registers the Enter, and dropping the tag there strands it — both deliver
// rules would ignore it forever. Stale tags are harmless; the next load
// overwrites, and unload clears explicitly.
func pruneAPCCargoIntent(memory map[string]any, units []model.Unit) {
	intent := GetAPCCargoIntent(memory)
	if len(intent) == 0 {
		return
	}
	live := make(map[int]bool, len(units))
	for _, u := range units {
		live[u.ID] = true
	}
	for id := range intent {
		if !live[id] {
			delete(intent, id)
		}
	}
}

// ClearAPCCargoIntent frees an APC to be re-tagged on its next load.
func ClearAPCCargoIntent(memory map[string]any, id int) {
	delete(GetAPCCargoIntent(memory), id)
}

// IdleEngineerLoadedAPCs returns loaded APCs tagged as carrying engineers.
func (e RuleEnv) IdleEngineerLoadedAPCs() []model.Unit {
	return e.idleLoadedAPCsByIntent(apcIntentEngineer)
}

// IdleCombatLoadedAPCs returns loaded APCs tagged as carrying combat infantry.
func (e RuleEnv) IdleCombatLoadedAPCs() []model.Unit {
	return e.idleLoadedAPCsByIntent(apcIntentCombat)
}

func (e RuleEnv) idleLoadedAPCsByIntent(want string) []model.Unit {
	pruneAPCCargoIntent(e.Memory, e.State.Units)
	intent := GetAPCCargoIntent(e.Memory)
	var out []model.Unit
	for _, u := range e.State.Units {
		if !u.Idle || !isTransport(u) || u.CargoCount == 0 {
			continue
		}
		if intent[u.ID] == want {
			out = append(out, u)
		}
	}
	return out
}

func (e RuleEnv) IdleEmptyAPCs() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && isTransport(u) && u.CargoCount == 0 {
			out = append(out, u)
		}
	}
	return out
}

// CanBuildTransport returns true if the faction can build any transport (APC or Ranger).
func (e RuleEnv) CanBuildTransport() bool {
	return e.CanBuildRole("apc") || e.CanBuildRole("ranger")
}

// TransportCount returns the total number of APCs and Rangers.
func (e RuleEnv) TransportCount() int {
	return e.RoleCount("apc") + e.RoleCount("ranger")
}

func (e RuleEnv) IdleRangers() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && matchesType(u.Type, Ranger) {
			out = append(out, u)
		}
	}
	return out
}

// getScoutID returns the designated scout light tank ID (0 = none).
func getScoutID(memory map[string]any) int {
	if v, ok := memory["scoutUnitID"].(int); ok {
		return v
	}
	return 0
}

// HasScout returns true if a scout unit (ranger or designated light tank) exists.
func (e RuleEnv) HasScout() bool {
	for _, u := range e.State.Units {
		if matchesType(u.Type, Ranger) {
			return true
		}
	}
	return getScoutID(e.Memory) != 0
}

// IdleScouts returns idle rangers plus the designated scout light tank (if idle).
func (e RuleEnv) IdleScouts() []model.Unit {
	scoutID := getScoutID(e.Memory)
	var out []model.Unit
	for _, u := range e.State.Units {
		if !u.Idle {
			continue
		}
		if matchesType(u.Type, Ranger) {
			out = append(out, u)
		} else if scoutID != 0 && u.ID == scoutID {
			out = append(out, u)
		}
	}
	return out
}

// IdleMinelayers excludes minelayers already assigned a minefield.
func (e RuleEnv) IdleMinelayers() []model.Unit {
	assigned := getMinelayerAssignments(e.Memory)
	var out []model.Unit
	for _, u := range e.State.Units {
		if u.Idle && matchesType(u.Type, Minelayer) && !assigned[u.ID] {
			out = append(out, u)
		}
	}
	return out
}

func getMinelayerAssignments(memory map[string]any) map[int]bool {
	if v, ok := memory["minelayerAssigned"].(map[int]bool); ok {
		return v
	}
	return make(map[int]bool)
}

// designateScout picks a scout, preferring a light tank for survivability and
// vision, then an attack dog — cheap, fast, disposable, and the only non-APC
// scouting a Soviet rush opening has. Called each tick so the designation
// survives production events.
func designateScout(env RuleEnv) {
	scoutID := getScoutID(env.Memory)
	if scoutID != 0 {
		for _, u := range env.State.Units {
			if u.ID == scoutID {
				return // still alive, keep designation
			}
		}
		delete(env.Memory, "scoutUnitID")
		scoutID = 0
	}
	// Rangers need no designation; IdleScouts already includes them all.
	for _, u := range env.State.Units {
		if matchesType(u.Type, Ranger) {
			return
		}
	}
	assigned := squadUnitIDSet(env.Memory)
	// The dog goes first: it is the disposable one, and a scout is spent
	// sooner or later. The first dog patrols, later ones guard base.
	for _, u := range env.State.Units {
		if u.Idle && matchesType(u.Type, AttackDog) && !assigned[u.ID] {
			env.Memory["scoutUnitID"] = u.ID
			slog.Debug("designated scout attack dog", "id", u.ID)
			return
		}
	}
	// A light tank scouts only when the armour can spare one. Game 102 fielded
	// a peak of three combat vehicles and put its only light tank on patrol,
	// where it died; an army that small cannot pay for the vision.
	if env.CombatVehicleCount() <= lightTankScoutMinVehicles {
		return
	}
	for _, u := range env.State.Units {
		if u.Idle && matchesType(u.Type, LightTank) && !assigned[u.ID] {
			env.Memory["scoutUnitID"] = u.ID
			slog.Debug("designated scout light tank", "id", u.ID)
			return
		}
	}
}

// IdleCombatInfantry excludes engineers — they have their own capture workflow.
func (e RuleEnv) IdleCombatInfantry() []model.Unit {
	var out []model.Unit
	for _, u := range e.State.Units {
		if !u.Idle {
			continue
		}
		if isInfantry(u) && !matchesType(u.Type, Engineer) {
			out = append(out, u)
		}
	}
	return out
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

// GroundUnitCentroid locates our own cluster, for targeting the iron curtain.
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
