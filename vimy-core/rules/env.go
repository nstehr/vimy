package rules

import (
	"log/slog"
	"math"
	"slices"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
)

// UnitPreferences holds per-category ordered role lists set by the LLM.
// BestBuildable* functions check these before falling back to hardcoded priority.
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
	Terrain     *model.TerrainGrid
	Preferences UnitPreferences
	TargetBias  TargetBias
}

func biasOr1(b float64) float64 {
	if b == 0 {
		return 1.0
	}
	return b
}

func (e RuleEnv) HasUnit(t string) bool      { return containsType(e.State.Units, t) }
func (e RuleEnv) HasBuilding(t string) bool  { return containsType(e.State.Buildings, t) }
func (e RuleEnv) UnitCount(t string) int     { return countType(e.State.Units, t) }
func (e RuleEnv) BuildingCount(t string) int { return countType(e.State.Buildings, t) }

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

// QueueProducingRole returns true if any queue for the given role is currently
// building or has queued an item that matches the role. This catches cases
// where QueueBusy returns false (e.g. item at 100%, or a second queue is free)
// but the role is already in production.
func (e RuleEnv) QueueProducingRole(name string) bool {
	r, ok := roles[name]
	if !ok {
		return false
	}
	for _, pq := range e.State.ProductionQueues {
		if !strings.EqualFold(pq.Type, r.queue) {
			continue
		}
		// Check current item.
		for _, t := range r.types {
			if matchesType(pq.CurrentItem, t) {
				return true
			}
		}
		// Check queued items.
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
// Net, not gross: it is negative or zero exactly when every credit is being
// committed as it arrives, which is the pathology the savings model exists to
// answer. Game 85 ran at cash 0 with a 500-tick delta of 0 while its build
// queue never paused. A gross figure would report a healthy economy there.
//
// Sampled by the engine rather than derived here, because a rule environment
// sees one tick and a rate needs two.
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

// allChokepoints runs FindChokepoints once per match and caches the result in
// memory. The terrain grid is static, so re-scanning each tick would be waste.
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

// ChokepointsTowardEnemy returns chokepoints ranked by relevance to the path
// between the base centroid and the nearest known enemy base. Falls back to
// the unranked list when there is no enemy intel so callers can still mine
// defensively on structure. Returns nil if no terrain or no chokes exist.
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

// DamagedSquadUnits returns idle squad members below the given HP threshold.
// Used by retreat rules to pull wounded units out of the fight.
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

// DamagedCombatUnits returns all combat units below the HP threshold,
// regardless of idle/squad status. Excludes harvesters, MCVs, rangers,
// engineers, APCs, designated scouts (non-combat/utility). Skips units
// already retreating.
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

// getRetreatingUnits returns the set of unit IDs currently retreating.
// Values are the tick when the retreat started (used for timeout).
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

// isInfantry returns true for infantry-class units.
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

// ServiceDepotOrCentroid returns the position of the service depot, or the
// building centroid if none exists. Falls back to (0, 0).
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

// AircraftCapacity returns the physical aircraft slot count from our pads:
// 4 per Airfield (yak/MiG bays) plus 1 per Helipad (single helicopter pad).
// Used to gate produce-aircraft so we don't queue planes beyond available
// pads — without this, completed aircraft sit at 100% with nowhere to
// spawn and cancel-stuck-aircraft just churns the queue (vimy-rmb follow-up
// from game 29: 120 cancel-stuck firings on 137 produce-aircraft, ~87%
// of production wasted).
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

// IsRushed reports whether the strategist's pressure detector has flagged
// this match as an early-game rush (small base + recent harvester pressure
// before tick ~10500). Production rules can use this to drop savings
// reserves and lower cash floors so cheap defenders spawn faster — without
// the prompt-side rush response can't translate into actual unit output.
func (e RuleEnv) IsRushed() bool {
	v, _ := e.Memory["beingRushed"].(bool)
	return v
}

// IsHarvesterHarassed reports whether the strategist has flagged sustained
// harvester pressure (mid-game, established base, 2+ harvester events in
// the last ~2000 ticks). Available for rules that want to differentiate
// rush-time vs. raid-time responses.
func (e RuleEnv) IsHarvesterHarassed() bool {
	v, _ := e.Memory["harvesterHarassed"].(bool)
	return v
}

// SquadClumped reports whether ≥80% of the named squad's living members
// are within `radiusCells` map cells of the squad centroid. Used by the
// rally-then-attack behavior so a fresh squad-attack fires only when the
// squad has actually assembled — arriving together concentrates damage,
// versus stringing out and getting picked off one at a time.
func (e RuleEnv) SquadClumped(name string, radiusCells int) bool {
	squads, ok := e.Memory["squads"].(map[string]*Squad)
	if !ok {
		return true // no squad map — trivially "clumped"
	}
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return true // no squad — no dispersion to worry about
	}

	ids := make(map[int]bool, len(sq.UnitIDs))
	for _, id := range sq.UnitIDs {
		ids[id] = true
	}
	var sumX, sumY int
	var members []model.Unit
	for _, u := range e.State.Units {
		if !ids[u.ID] {
			continue
		}
		sumX += u.X
		sumY += u.Y
		members = append(members, u)
	}
	if len(members) < 2 {
		return true // one unit is trivially clumped
	}
	cx := sumX / len(members)
	cy := sumY / len(members)

	radiusSq := radiusCells * radiusCells
	near := 0
	for _, u := range members {
		dx := u.X - cx
		dy := u.Y - cy
		if dx*dx+dy*dy <= radiusSq {
			near++
		}
	}
	// 80% threshold.
	return near*10 >= len(members)*8
}

// AxisBurned reports whether the strategist has marked a unit axis (air,
// infantry, vehicle, naval) as burned — i.e. the doctrine has pivoted to
// this axis 2+ times this match and each pivot was followed by domain-
// specific counter events. Production rules can gate on this so the bot
// stops feeding a hard-countered domain regardless of what doctrine the
// LLM picks. Set by the strategist after computeBurnedAxes runs each
// evaluation (vimy-w13 follow-up: promoting burned axes from soft prompt
// constraint to a hard rule-engine gate after game 27 showed aircraft
// kept being committed despite 4 separate Flak counter events).
func (e RuleEnv) AxisBurned(axis string) bool {
	m, ok := e.Memory["burnedAxes"].(map[string]bool)
	if !ok {
		return false
	}
	return m[axis]
}

// OverextendedSquadMembers returns idle squad members whose distance from
// base centroid exceeds leashPct fraction of the map diagonal AND who are
// not making forward progress toward any known enemy base. A unit counts
// as forward-progressing when its distance to the nearest enemy base is
// less than its distance to our base centroid — that catches:
//   - units staging at a BestApproachAxis flank waypoint (vimy-b14)
//   - units in transit toward the enemy
//   - units mid-engagement at the enemy base
//
// Without the exemption (or with a too-tight enemy-base radius), the recall
// cycle yanks staged units back to base, where they re-form into the squad,
// get attack-moved toward the waypoint again, arrive idle, get recalled,
// and so on. Game 23 (126k-tick loss) had 764 recall firings before the
// initial enemy-base exemption (vimy-91b); game 29 had 554 firings after,
// because the 20% radius didn't cover the flank waypoints introduced by
// vimy-b14. The 'forward progress' rule (vimy-rmb) generalizes the
// exemption to cover staging anywhere closer to the enemy than to home.
func (e RuleEnv) OverextendedSquadMembers(name string, leashPct float64) []model.Unit {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return nil
	}
	centX, centY := e.BuildingCentroid()
	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	diagonal := math.Sqrt(mw*mw + mh*mh)
	leashDist := diagonal * leashPct
	leashSq := leashDist * leashDist
	bases := getEnemyBases(e.Memory)

	idleSet := make(map[int]bool)
	unitMap := make(map[int]model.Unit)
	for _, u := range e.State.Units {
		unitMap[u.ID] = u
		if u.Idle {
			idleSet[u.ID] = true
		}
	}

	var out []model.Unit
	for _, id := range sq.UnitIDs {
		if !idleSet[id] {
			continue
		}
		u := unitMap[id]
		dx := float64(u.X - centX)
		dy := float64(u.Y - centY)
		distToHomeSq := dx*dx + dy*dy
		if distToHomeSq <= leashSq {
			continue
		}
		// Forward-progress exemption: if the unit is closer to any known
		// enemy base than to our base centroid, treat it as in-transit or
		// staging — not overextended.
		forwardProgressing := false
		for _, b := range bases {
			ex := float64(u.X - b.X)
			ey := float64(u.Y - b.Y)
			if ex*ex+ey*ey < distToHomeSq {
				forwardProgressing = true
				break
			}
		}
		if forwardProgressing {
			continue
		}
		out = append(out, u)
	}
	return out
}

// SquadThreatRatio computes (total enemy HP near squad centroid) / (total squad HP).
// A ratio > 1.0 means enemies have more HP than us locally. radiusPct is a
// fraction of the map diagonal.
func (e RuleEnv) SquadThreatRatio(name string, radiusPct float64) float64 {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return 0
	}
	unitMap := make(map[int]model.Unit)
	for _, u := range e.State.Units {
		unitMap[u.ID] = u
	}
	sumX, sumY, squadHP, n := 0, 0, 0, 0
	for _, id := range sq.UnitIDs {
		if u, ok := unitMap[id]; ok {
			sumX += u.X
			sumY += u.Y
			squadHP += u.HP
			n++
		}
	}
	if n == 0 || squadHP == 0 {
		return 0
	}
	cx, cy := sumX/n, sumY/n

	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	radius := math.Sqrt(mw*mw+mh*mh) * radiusPct
	radiusSq := radius * radius

	enemyHP := 0
	for _, en := range e.State.Enemies {
		dx := float64(en.X - cx)
		dy := float64(en.Y - cy)
		if dx*dx+dy*dy <= radiusSq {
			enemyHP += en.HP
		}
	}
	if enemyHP == 0 {
		return 0
	}
	return float64(enemyHP) / float64(squadHP)
}

// WeakestVisibleEnemy returns the enemy with the lowest HP/MaxHP ratio.
// Skips enemies with MaxHP == 0. Breaks ties by proximity to first building.
func (e RuleEnv) WeakestVisibleEnemy() *model.Enemy {
	if len(e.State.Enemies) == 0 {
		return nil
	}
	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}
	var weakest *model.Enemy
	bestRatio := 2.0 // above max possible ratio of 1.0
	bestDist := math.MaxFloat64
	for i := range e.State.Enemies {
		en := &e.State.Enemies[i]
		if en.MaxHP == 0 {
			continue
		}
		ratio := float64(en.HP) / float64(en.MaxHP)
		dx := float64(en.X - bx)
		dy := float64(en.Y - by)
		dist := dx*dx + dy*dy
		if ratio < bestRatio || (ratio == bestRatio && dist < bestDist) {
			bestRatio = ratio
			bestDist = dist
			weakest = en
		}
	}
	return weakest
}

// HarvesterThreatRatio is how much enemy strength, relative to a harvester's own
// health, counts as danger.
//
// Presence alone does not: a lone rifleman used to be indistinguishable from a
// tank column, and one of them parked the economy for 17,000 ticks of game 71 —
// flee-harvesters fired 696 times while the harvesters shuttled to the refinery
// and back without ever filling up (vimy-mfq).
//
// Relative rather than an absolute HP figure, matching SquadThreatRatio, so it
// needs no table of unit health and does not drift when the mod changes one.
const HarvesterThreatRatio = 0.5

// HarvestersInDanger returns the harvesters (idle or not) with enough enemy
// strength within danger range to be worth running from. dangerPct is a
// fraction of the map diagonal.
func (e RuleEnv) HarvestersInDanger(dangerPct float64) []model.Unit {
	if len(e.State.Enemies) == 0 {
		return nil
	}
	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	threshold := math.Sqrt(mw*mw+mh*mh) * dangerPct
	threshSq := threshold * threshold

	var out []model.Unit
	for _, u := range e.State.Units {
		if !matchesType(u.Type, Harvester) {
			continue
		}
		// Summed, so several small units are a threat even though one is not.
		nearbyHP := 0
		for _, en := range e.State.Enemies {
			dx := float64(u.X - en.X)
			dy := float64(u.Y - en.Y)
			if dx*dx+dy*dy < threshSq {
				nearbyHP += en.HP
			}
		}
		if nearbyHP == 0 {
			continue
		}
		// Against the harvester's full health rather than its current: a
		// harvester already down to a sliver should not become easier to spook
		// as it takes damage.
		own := u.MaxHP
		if own <= 0 {
			own = u.HP
		}
		if own <= 0 || float64(nearbyHP) >= float64(own)*HarvesterThreatRatio {
			out = append(out, u)
		}
	}
	return out
}

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
// (harvesters, MCVs), utility units (engineers, APCs, minelayers, rangers),
// attack dogs (bite-only, anti-infantry — shouldn't be dispatched against
// vehicles by offensive squads), and other domains (aircraft, naval).
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

// NearBaseGroundUnits returns ground combat units near any building, regardless
// of idle status. Used for emergency base defense so units with active orders
// (e.g. en route to an attack) are recalled when the base is under attack.
// Uses the same 20% map-diagonal threshold as BaseUnderAttack.
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

// APC cargo intent: the game state only tells us an APC has N passengers, not
// what kind. We tag each APC as "engineer" or "combat" at load time so the
// deliver rules (capture vs. assault) only pick APCs carrying their own kind.
const (
	apcIntentEngineer = "engineer"
	apcIntentCombat   = "combat"
)

// GetAPCCargoIntent returns the intent map, allocating if needed. Exported for
// actions that need to tag on load or clear on unload.
func GetAPCCargoIntent(memory map[string]any) map[int]string {
	return memoryMap[int, string](memory, "apcCargoIntent")
}

// pruneAPCCargoIntent drops entries for APCs that no longer exist. It does
// NOT prune based on CargoCount: a freshly-tagged APC takes a tick or two
// for the server to register the Enter command, during which the APC shows
// CargoCount=0 on our side. Pruning on "empty" would drop the tag during
// this transient, and both deliver rules would then ignore the loaded APC
// forever. Stale tags from failed loads are harmless — the next successful
// load overwrites. Unload paths clear the tag explicitly via ClearAPCCargoIntent.
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

// ClearAPCCargoIntent removes the intent tag for a specific APC. Call on
// unload so the APC can be re-tagged when it picks up a new passenger.
func ClearAPCCargoIntent(memory map[string]any, id int) {
	delete(GetAPCCargoIntent(memory), id)
}

// IdleEngineerLoadedAPCs returns loaded APCs tagged as carrying engineers.
// Used by deliver-apc-to-target so it won't pick up a combat-loaded APC.
func (e RuleEnv) IdleEngineerLoadedAPCs() []model.Unit {
	return e.idleLoadedAPCsByIntent(apcIntentEngineer)
}

// IdleCombatLoadedAPCs returns loaded APCs tagged as carrying combat infantry.
// Used by deliver-assault-apc so it won't pick up an engineer-loaded APC.
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

// IdleMinelayers returns idle minelayer units that haven't been given a
// minefield order yet (tracked via memory to avoid re-issuing every tick).
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

// designateScout assigns the first unassigned idle scout-capable unit. A light
// tank is preferred when available (more survivable, more vision). Falls back
// to an attack dog when no light tank exists — the Soviet equivalent of the
// Allied Ranger: cheap, fast, disposable. Humans use the first dog out of the
// kennel as their perimeter scout in Soviet rush openings, and before this
// fallback Soviet rush doctrines had no non-APC scouting at all.
// Called each tick from the engine so the designation persists across
// production events.
func designateScout(env RuleEnv) {
	scoutID := getScoutID(env.Memory)
	if scoutID != 0 {
		// Check if the designated scout is still alive.
		for _, u := range env.State.Units {
			if u.ID == scoutID {
				return // still alive, keep designation
			}
		}
		// Scout died — clear designation.
		delete(env.Memory, "scoutUnitID")
		scoutID = 0
	}
	// If we have rangers, no need for a dedicated designation — IdleScouts
	// includes all rangers automatically.
	for _, u := range env.State.Units {
		if matchesType(u.Type, Ranger) {
			return
		}
	}
	assigned := squadUnitIDSet(env.Memory)
	// Prefer a light tank.
	for _, u := range env.State.Units {
		if u.Idle && matchesType(u.Type, LightTank) && !assigned[u.ID] {
			env.Memory["scoutUnitID"] = u.ID
			slog.Debug("designated scout light tank", "id", u.ID)
			return
		}
	}
	// Soviet fallback: first unassigned idle attack dog. Matches the human
	// pattern of sending the first dog on patrol while later dogs guard base.
	for _, u := range env.State.Units {
		if u.Idle && matchesType(u.Type, AttackDog) && !assigned[u.ID] {
			env.Memory["scoutUnitID"] = u.ID
			slog.Debug("designated scout attack dog", "id", u.ID)
			return
		}
	}
}

func (e RuleEnv) CapturableCount() int { return len(e.State.Capturables) }

// capturableValue assigns a strategic value to capturable building types.
// Higher value = more desirable target. Unknown types get a baseline score.
var capturableValue = map[string]float64{
	"oilb": 10, // Oil derrick: continuous cash income
	"fcom": 8,  // Forward command: expands build area
	"miss": 5,  // Communications center: large radar reveal
	"bio":  4,  // Bio lab: provides prerequisite
	"hosp": 3,  // Hospital: heals infantry
}

const capturableValueDefault = 2 // unknown capturable types

func (e RuleEnv) NearestCapturable() *model.Enemy {
	return e.BestCapturable()
}

// BestCapturable picks the highest-value capturable, using distance as a
// tiebreaker. Value is divided by sqrt(distance) so nearby low-value targets
// can still beat distant high-value ones when the trip cost is too high.
func (e RuleEnv) BestCapturable() *model.Enemy {
	if len(e.State.Capturables) == 0 {
		return nil
	}
	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}
	var best *model.Enemy
	bestScore := -1.0
	for i := range e.State.Capturables {
		c := &e.State.Capturables[i]
		// Skip water/cliff capturables — engineers are ground units.
		if e.Terrain != nil {
			t := e.Terrain.AtMapPos(c.X, c.Y)
			if t != model.Land && t != model.Bridge {
				continue
			}
		}
		dx := float64(c.X - bx)
		dy := float64(c.Y - by)
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist < 1 {
			dist = 1
		}
		val := capturableValue[strings.ToLower(c.Type)]
		if val == 0 {
			val = capturableValueDefault
		}
		score := val / math.Sqrt(dist)
		if score > bestScore {
			bestScore = score
			best = c
		}
	}
	return best
}

// EngineerNearCapturable returns true if any idle engineer is within capture
// range of a capturable building (e.g. just unloaded from an APC).
func (e RuleEnv) EngineerNearCapturable() bool {
	engineers := e.IdleEngineers()
	if len(engineers) == 0 {
		return false
	}
	for i := range e.State.Capturables {
		c := &e.State.Capturables[i]
		for _, eng := range engineers {
			dx := float64(eng.X - c.X)
			dy := float64(eng.Y - c.Y)
			if dx*dx+dy*dy < 8*8 {
				return true
			}
		}
	}
	return false
}

// airTargetValue assigns a strategic value to enemy types for air strikes.
// Higher value = more desirable target. Defense structures score highest
// because aircraft bypass ground defenses and can soften positions before
// a ground push.
var airTargetValue = map[string]float64{
	// Win-condition target — same reasoning as groundTargetValue (vimy-68x).
	ConstructionYard: 12, "afac": 12,
	// Defense structures — clear them so we can stay over the base.
	TeslaCoil: 10, Turret: 8, Pillbox: 7, CamoPillbox: 7, FlameTower: 6,
	// Superweapons
	MissileSilo: 9, IronCurtain: 9,
	// Production
	WarFactory: 5, Airfield: 5, Helipad: 5,
	SovietBarracks: 4, AlliedBarracks: 4,
	// Economy — bumped from 4 to 6 so refineries get pressed.
	Refinery: 6,
	// AA defenses (risky but worth removing)
	AAGun: 4, SAMSite: 4,
	// Power / support
	AdvancedPower: 3, PowerPlant: 2, RadarDome: 3, AlliedTechCenter: 3, SovietTechCenter: 3,
	SubPen: 4, NavalYard: 4,
}

const airTargetValueDefault = 1.0 // mobile units / unknown types

// BestAirTarget picks the highest-value enemy for air strikes, using distance
// as a decay factor. Scoring: val * hpBonus / sqrt(dist).
// val = type value from airTargetValue (dominant factor)
// hpBonus = 2.0 - hpRatio — gentle tiebreaker favoring damaged targets
// 1/sqrt(dist) = inverse distance to own base
func (e RuleEnv) BestAirTarget() *model.Enemy {
	if len(e.State.Enemies) == 0 {
		return nil
	}
	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}
	var best *model.Enemy
	bestScore := -1.0
	for i := range e.State.Enemies {
		en := &e.State.Enemies[i]
		if en.MaxHP == 0 {
			continue
		}
		// Strip faction suffix (e.g. "afld.ukraine" → "afld").
		base := strings.ToLower(en.Type)
		if idx := strings.IndexByte(base, '.'); idx >= 0 {
			base = base[:idx]
		}
		val := airTargetValue[base]
		if val == 0 {
			val = airTargetValueDefault
		}
		switch base {
		case TeslaCoil, Turret, Pillbox, CamoPillbox, FlameTower:
			val *= biasOr1(e.TargetBias.AirGroundDef)
		}
		hpRatio := float64(en.HP) / float64(en.MaxHP)
		hpBonus := 2.0 - hpRatio // 1.0 (full HP) to 2.0 (near-death)

		dx := float64(en.X - bx)
		dy := float64(en.Y - by)
		dist := math.Sqrt(dx*dx + dy*dy)
		if dist < 1 {
			dist = 1
		}
		score := val * hpBonus / math.Sqrt(dist)
		if score > bestScore {
			bestScore = score
			best = en
		}
	}
	return best
}

// groundTargetValue assigns a strategic value to enemy types for ground attacks.
// Active base defenses score highest because they're actively killing our ground
// units. AA defenses and naval buildings score low — not threatening to ground forces.
var groundTargetValue = map[string]float64{
	// Win-condition target: destroying the construction yard is the actual
	// objective. Game 19 (vimy-68x): 21 STRONG-rated doctrines, attack >
	// defense, but lost because squads kept fighting mobile units instead
	// of pressing enemy infrastructure. CY now leads the value table.
	ConstructionYard: 12,
	// Active base defenses — kill these first so the push survives.
	TeslaCoil: 10, Turret: 8, Pillbox: 7, CamoPillbox: 7, FlameTower: 6,
	// Superweapons
	MissileSilo: 9, IronCurtain: 9,
	// AA defenses (low threat to ground)
	AAGun: 2, SAMSite: 2,
	// Production (destroy their ability to replace losses)
	WarFactory: 6, Airfield: 5, Helipad: 5, SovietBarracks: 4, AlliedBarracks: 4,
	// Economy — bumped from 5 to 7 so refineries get pressed once
	// active defenses are down (economic strangulation is a real win path).
	Refinery: 7,
	// Naval (low priority for ground forces)
	SubPen: 2, NavalYard: 2,
	// Power / support
	AdvancedPower: 3, PowerPlant: 2, RadarDome: 3, AlliedTechCenter: 3, SovietTechCenter: 3,
}

const groundTargetValueDefault = 1.0 // mobile units / unknown types

// BestGroundTarget picks the highest-value enemy for ground attacks. Scoring:
// val * hpBonus / (1 + dist/groundDistanceScale).
// val      = type value from groundTargetValue (dominant factor)
// hpBonus  = 2.0 - hpRatio — gentle tiebreaker favoring damaged targets
// distance = soft decay so a high-value building across the map can still
//
//	beat a value-1 mobile unit at the squad's doorstep. The old
//	1/dist decay caused squads to chase trash units forever instead
//	of pressing enemy infrastructure (vimy-68x).
const groundDistanceScale = 50.0

func (e RuleEnv) BestGroundTarget() *model.Enemy {
	if len(e.State.Enemies) == 0 {
		return nil
	}
	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}
	var best *model.Enemy
	bestScore := -1.0
	for i := range e.State.Enemies {
		en := &e.State.Enemies[i]
		if en.MaxHP == 0 {
			continue
		}
		// Strip faction suffix (e.g. "afld.ukraine" → "afld").
		base := strings.ToLower(en.Type)
		if idx := strings.IndexByte(base, '.'); idx >= 0 {
			base = base[:idx]
		}
		val := groundTargetValue[base]
		if val == 0 {
			val = groundTargetValueDefault
		}
		switch base {
		case AAGun, SAMSite:
			val *= biasOr1(e.TargetBias.GroundAA)
		}
		hpRatio := float64(en.HP) / float64(en.MaxHP)
		hpBonus := 2.0 - hpRatio // 1.0 (full HP) to 2.0 (near-death)

		dx := float64(en.X - bx)
		dy := float64(en.Y - by)
		dist := math.Sqrt(dx*dx + dy*dy)
		score := val * hpBonus / (1 + dist/groundDistanceScale)
		if score > bestScore {
			bestScore = score
			best = en
		}
	}
	return best
}

// IdleCombatInfantry returns idle infantry excluding engineers (which have
// their own capture workflow). Used for transport assault loading.
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

func (e RuleEnv) SquadNeedsReinforcement(name string) bool {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return false
	}
	return len(sq.UnitIDs) < sq.TargetSize
}

// SquadReadyRatio returns the fraction of *available* squad members that are
// idle. Retreating/repairing units are excluded from both numerator and
// denominator so they don't block the ratio — a cautious doctrine waiting
// for 100% readiness can still attack with all healthy members while one
// unit heals at the depot.
func (e RuleEnv) SquadReadyRatio(name string) float64 {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return 0
	}
	idleSet := make(map[int]bool)
	for _, u := range e.State.Units {
		if u.Idle {
			idleSet[u.ID] = true
		}
	}
	retreating := getRetreatingUnits(e.Memory)
	idle, available := 0, 0
	for _, id := range sq.UnitIDs {
		if _, isRetreating := retreating[id]; isRetreating {
			continue
		}
		available++
		if idleSet[id] {
			idle++
		}
	}
	if available == 0 {
		return 0
	}
	return float64(idle) / float64(available)
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
	retreating := getRetreatingUnits(e.Memory)
	n := 0
	for _, id := range sq.UnitIDs {
		_, isRetreating := retreating[id]
		if idleSet[id] && !isRetreating {
			n++
		}
	}
	return n
}

// SquadAwayFromBase returns true if the squad centroid is further than
// radiusPct (fraction of map diagonal) from the building centroid.
// Used to prevent disengage from firing when the squad is already home.
func (e RuleEnv) SquadAwayFromBase(name string, radiusPct float64) bool {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return false
	}
	unitMap := make(map[int]model.Unit)
	for _, u := range e.State.Units {
		unitMap[u.ID] = u
	}
	sumX, sumY, n := 0, 0, 0
	for _, id := range sq.UnitIDs {
		if u, ok := unitMap[id]; ok {
			sumX += u.X
			sumY += u.Y
			n++
		}
	}
	if n == 0 {
		return false
	}
	sx, sy := float64(sumX/n), float64(sumY/n)
	bx, by := e.BuildingCentroid()
	dx := sx - float64(bx)
	dy := sy - float64(by)
	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	radius := math.Sqrt(mw*mw+mh*mh) * radiusPct
	return dx*dx+dy*dy > radius*radius
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

// enemyHarvesterIntel persists last-known positions of enemy harvesters.
// Keyed by harvester ID. Harvesters cluster near refineries which sit
// inside enemy bases, so their positions remain useful location intel
// even after they fade back into fog.
type enemyHarvesterIntel struct {
	X, Y, Tick int
}

// IntelCenter is an inferred estimate of the enemy base position based on
// the full set of accumulated sightings — visible buildings and harvesters
// plus persistent high-confidence base intel. Confidence scales 0..1 with
// the total weighted sighting mass; a lone harvester sighting yields low
// confidence, multiple buildings yield high.
type IntelCenter struct {
	X, Y       int
	Confidence float64
	Sources    int
}

// knownBuildingTypes distinguishes enemy buildings from mobile units in the
// Enemies list. Building sightings give high-confidence base positions;
// unit sightings might just be an attack force passing through.
var knownBuildingTypes = map[string]bool{
	// Production
	ConstructionYard: true, SovietBarracks: true, AlliedBarracks: true, WarFactory: true, Kennel: true,
	Airfield: true, Helipad: true, NavalYard: true, SubPen: true,
	// Economy
	PowerPlant: true, AdvancedPower: true, Refinery: true, OreSilo: true,
	// Tech / Support
	RadarDome: true, AlliedTechCenter: true, SovietTechCenter: true, ServiceDepot: true,
	// Superweapons
	IronCurtain: true, "pdox": true, MissileSilo: true, GapGenerator: true,
	// Defenses
	Pillbox: true, CamoPillbox: true, Turret: true, FlameTower: true,
	TeslaCoil: true, AAGun: true, SAMSite: true,
}

// IsKnownBuildingType handles faction variants (e.g. "afld.ukraine" → "afld").
func IsKnownBuildingType(t string) bool {
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
		if IsKnownBuildingType(e.Type) {
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

	// Clear stale intel: when our units have been near a known enemy base
	// position for a sustained period without seeing enemies, the base has
	// been genuinely cleared. Prior behavior (single-tick check + 300-tick
	// minAge) let a scout passing through wipe intel — game observed live:
	// scout finds enemy at tick 6520, intel set, scout rotates back through
	// the area 30 seconds later, at that instant enemy is behind fog →
	// intel cleared → squad-attack-known-base can't fire → 6-rifle squad
	// sits at home. Fix: (1) intelMinAge 300 → 3000 (intel persists past
	// the rush window even without visits), (2) require intelClearSustain
	// ticks of continuous "our unit near + no enemy visible" before clearing.
	const intelClearRadius = 10
	const intelMinAge = 3000
	const intelClearSustain = 500 // ticks of continuous confirmation
	intelClearRadiusSq := intelClearRadius * intelClearRadius

	sustain := memoryMap[string, int](env.Memory, "enemyBaseIntelClearSustain")

	for owner, intel := range bases {
		if !intel.FromBuildings {
			continue // only clear high-confidence intel
		}
		if env.State.Tick-intel.Tick < intelMinAge {
			continue // too fresh — could be behind fog of war
		}
		// Check: any of our units near this intel position?
		ourUnitNearby := false
		for _, u := range env.State.Units {
			dx := u.X - intel.X
			dy := u.Y - intel.Y
			if dx*dx+dy*dy < intelClearRadiusSq {
				ourUnitNearby = true
				break
			}
		}
		if !ourUnitNearby {
			delete(sustain, owner) // reset counter when we leave
			continue
		}
		// Our units are there — check if any enemies visible nearby.
		enemyNearby := false
		for _, e := range env.State.Enemies {
			dx := e.X - intel.X
			dy := e.Y - intel.Y
			if dx*dx+dy*dy < intelClearRadiusSq {
				enemyNearby = true
				break
			}
		}
		if enemyNearby {
			delete(sustain, owner) // reset counter when enemies appear
			continue
		}
		// Sustain counter: track when this "our unit near + no enemy" run started.
		startTick, ok := sustain[owner]
		if !ok {
			sustain[owner] = env.State.Tick
			continue
		}
		if env.State.Tick-startTick < intelClearSustain {
			continue // not sustained long enough yet
		}
		slog.Info("clearing stale enemy base intel", "owner", owner, "x", intel.X, "y", intel.Y, "sustainTicks", env.State.Tick-startTick)
		delete(bases, owner)
		delete(sustain, owner)
	}

	env.Memory["enemyBases"] = bases

	// Persist last-known positions of enemy harvesters for InferredEnemyBaseCenter.
	harvesters := memoryMap[int, enemyHarvesterIntel](env.Memory, "enemyHarvesterIntel")
	for _, e := range env.State.Enemies {
		if matchesType(e.Type, Harvester) {
			harvesters[e.ID] = enemyHarvesterIntel{X: e.X, Y: e.Y, Tick: env.State.Tick}
		}
	}

	updateDefenseIntel(env)

	// Accumulate historical enemy sightings.
	// Units: deduplicate by ID — each unit is unique.
	// Buildings: track high-water mark — max visible at once per type,
	// since buildings get destroyed and rebuilt with new IDs.
	seenIDs := getEnemySeenIDs(env.Memory)
	unitsSeen := GetEnemyUnitsSeen(env.Memory)
	buildingsSeen := GetEnemyBuildingsSeen(env.Memory)

	curBuildingCounts := make(map[string]int)
	for _, e := range env.State.Enemies {
		if IsKnownBuildingType(e.Type) {
			curBuildingCounts[e.Type]++
		} else {
			if seenIDs[e.ID] {
				continue
			}
			seenIDs[e.ID] = true
			unitsSeen[e.Type]++
		}
	}
	for t, c := range curBuildingCounts {
		if c > buildingsSeen[t] {
			buildingsSeen[t] = c
		}
	}
	env.Memory["enemySeenIDs"] = seenIDs
	env.Memory["enemyUnitsSeen"] = unitsSeen
	env.Memory["enemyBuildingsSeen"] = buildingsSeen
}

// GetEnemyUnitsSeen returns the cumulative count of enemy units observed by type.
func GetEnemyUnitsSeen(memory map[string]any) map[string]int {
	if v, ok := memory["enemyUnitsSeen"].(map[string]int); ok {
		return v
	}
	return make(map[string]int)
}

// GetEnemyBuildingsSeen returns the cumulative count of enemy buildings observed by type.
func GetEnemyBuildingsSeen(memory map[string]any) map[string]int {
	if v, ok := memory["enemyBuildingsSeen"].(map[string]int); ok {
		return v
	}
	return make(map[string]int)
}

func getEnemySeenIDs(memory map[string]any) map[int]bool {
	if v, ok := memory["enemySeenIDs"].(map[int]bool); ok {
		return v
	}
	return make(map[int]bool)
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

// InferredEnemyBaseCenter returns the weighted centroid of all accumulated
// enemy-position intel — visible buildings, visible harvesters, persistent
// harvester last-known positions, and persistent high-confidence base intel.
// Weights: persistent base centroid = 5.0 (strongest; already aggregated from
// building sightings), currently-visible buildings = 3.0 each, harvesters
// = 1.0 each (decayed to 0.5 past 500 ticks to discount stale positions).
// Returns nil when no intel exists yet. Siege / artillery / scout-rally rules
// can target this point even when only fragments of the base have been seen.
func (e RuleEnv) InferredEnemyBaseCenter() *IntelCenter {
	sumX, sumY, totalW := 0.0, 0.0, 0.0
	sources := 0

	for _, enemy := range e.State.Enemies {
		var w float64
		switch {
		case IsKnownBuildingType(enemy.Type):
			w = 3.0
		case matchesType(enemy.Type, Harvester):
			w = 1.0
		}
		if w == 0 {
			continue
		}
		sumX += float64(enemy.X) * w
		sumY += float64(enemy.Y) * w
		totalW += w
		sources++
	}

	// Persistent harvester intel — keep contributing even after losing sight.
	for _, harv := range memoryMap[int, enemyHarvesterIntel](e.Memory, "enemyHarvesterIntel") {
		w := 1.0
		if e.State.Tick-harv.Tick > 500 {
			w = 0.5
		}
		sumX += float64(harv.X) * w
		sumY += float64(harv.Y) * w
		totalW += w
		sources++
	}

	// Persistent high-confidence base centroid — strongest single anchor.
	for _, base := range getEnemyBases(e.Memory) {
		if !base.FromBuildings {
			continue
		}
		sumX += float64(base.X) * 5.0
		sumY += float64(base.Y) * 5.0
		totalW += 5.0
		sources++
	}

	if totalW == 0 {
		return nil
	}
	conf := totalW / 15.0 // 15 ≈ three full-weight buildings
	if conf > 1.0 {
		conf = 1.0
	}
	return &IntelCenter{
		X:          int(sumX / totalW),
		Y:          int(sumY / totalW),
		Confidence: conf,
		Sources:    sources,
	}
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

// EnemyDefenseIntel is a remembered enemy defense building. We retain these
// across ticks so path planning can avoid defended approaches even when the
// defenses aren't currently in vision.
type EnemyDefenseIntel struct {
	ActorID int
	Type    string
	X, Y    int
	Tick    int
}

// defenseThreatWeight maps defense type → threat weight painted onto the
// field. Tesla > pillbox by rough DPS/range heuristic.
var defenseThreatWeight = map[string]float64{
	Pillbox:     1.0,
	CamoPillbox: 1.0,
	Turret:      1.2,
	FlameTower:  1.4,
	TeslaCoil:   2.5,
	AAGun:       0.4, // anti-air, not ground threat
	SAMSite:     0.4,
}

func isEnemyDefenseType(t string) bool {
	_, ok := defenseThreatWeight[baseTypeName(t)]
	return ok
}

func baseTypeName(t string) string {
	base := strings.ToLower(t)
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	return base
}

// updateDefenseIntel refreshes remembered defense positions. Currently visible
// defenses overwrite prior records; stale records (≥300 ticks old) are
// cleared if any of our units stand near the recorded spot and the defense
// isn't visible — mirrors the base-intel clearing pattern.
func updateDefenseIntel(env RuleEnv) {
	defs := getEnemyDefenses(env.Memory)
	for _, e := range env.State.Enemies {
		if !isEnemyDefenseType(e.Type) {
			continue
		}
		defs[e.ID] = EnemyDefenseIntel{
			ActorID: e.ID, Type: baseTypeName(e.Type), X: e.X, Y: e.Y, Tick: env.State.Tick,
		}
	}

	const clearRadiusSq = 10 * 10
	const minAge = 300
	for id, intel := range defs {
		if env.State.Tick-intel.Tick < minAge {
			continue
		}
		ourNearby := false
		for _, u := range env.State.Units {
			dx, dy := u.X-intel.X, u.Y-intel.Y
			if dx*dx+dy*dy < clearRadiusSq {
				ourNearby = true
				break
			}
		}
		if !ourNearby {
			continue
		}
		stillThere := false
		for _, e := range env.State.Enemies {
			if e.ID == intel.ActorID {
				stillThere = true
				break
			}
		}
		if !stillThere {
			delete(defs, id)
		}
	}
	env.Memory["enemyDefenses"] = defs
}

func getEnemyDefenses(memory map[string]any) map[int]EnemyDefenseIntel {
	if v, ok := memory["enemyDefenses"].(map[int]EnemyDefenseIntel); ok {
		return v
	}
	return make(map[int]EnemyDefenseIntel)
}

// ThreatField rasterizes remembered enemy defenses onto a field parallel to
// the terrain grid. Returns nil if no terrain grid is available.
func (e RuleEnv) ThreatField() *model.ThreatField {
	if e.Terrain == nil {
		return nil
	}
	f := model.NewThreatField(e.Terrain)
	for _, d := range getEnemyDefenses(e.Memory) {
		w, ok := defenseThreatWeight[d.Type]
		if !ok {
			w = 1.0
		}
		f.AddSource(e.Terrain, d.X, d.Y, w)
	}
	return f
}

// airDefenseThreatWeight is the AA-only threat weighting used for routing
// aircraft. Pillbox/Tesla/FlameTower don't shoot aircraft, so they shouldn't
// influence air approach. Only AAGun and SAMSite count. Weights are higher
// than ground equivalents because aircraft can't return fire as
// straightforwardly and a SAM cluster reliably shreds approaching strike
// packages.
var airDefenseThreatWeight = map[string]float64{
	AAGun:   2.0,
	SAMSite: 2.5,
}

// AirThreatField is the AA-only counterpart of ThreatField — used for routing
// aircraft around SAM/AAGun clusters before they engage a ground target.
func (e RuleEnv) AirThreatField() *model.ThreatField {
	if e.Terrain == nil {
		return nil
	}
	f := model.NewThreatField(e.Terrain)
	for _, d := range getEnemyDefenses(e.Memory) {
		w, ok := airDefenseThreatWeight[d.Type]
		if !ok {
			continue
		}
		f.AddSource(e.Terrain, d.X, d.Y, w)
	}
	return f
}

// ApproachWaypoint returns an intermediate attack-move target that avoids
// defended zones on the way to `dest`. It is the last zone on the weighted
// path whose cumulative threat is still low — where the squad should stage
// before committing to the final push. Returns zero, false when no waypoint
// is useful (open approach, no intel, unreachable).
func (e RuleEnv) ApproachWaypoint(destX, destY int) (int, int, bool) {
	return e.approachWaypointWithField(destX, destY, e.ThreatField())
}

// AirApproachWaypoint mirrors ApproachWaypoint but routes around AA defenses
// only. Used by air-strike squads so aircraft enter from a low-AA flank
// instead of flying straight over a SAM cluster.
func (e RuleEnv) AirApproachWaypoint(destX, destY int) (int, int, bool) {
	return e.approachWaypointWithField(destX, destY, e.AirThreatField())
}

// approachAxisRadiusCells is how far from the target the candidate flank
// waypoints sit. ~8 zone-cells is a small detour relative to base-to-target
// distance but well outside a defense cluster's painted radius, so the
// candidate east of the target scores cleanly even when defenses are
// painted south of the target.
const approachAxisRadiusCells = 8

// openEnoughThreshold gates the entire mechanism: if the direct corridor
// from our base to the target has less threat than this, don't pick a
// detour at all — the path is already open. Calibrated to roughly the
// signal of 1-2 distant defenses in the corridor.
const openEnoughThreshold = 1.0

// BestApproachAxis evaluates 8 compass-direction waypoints arranged around
// `dest` and returns the one whose corridor-from-base has the lowest threat.
// Unlike ApproachWaypoint (which picks a single weighted BFS path), this
// reliably picks the genuinely open flank when an enemy base has defenses
// on one side and a clear approach on the other. Returns false when:
//   - terrain or threat data is missing
//   - the direct corridor is already low-threat (open approach)
//   - no candidate is meaningfully cleaner than the direct approach
//
// Replaces ApproachWaypoint as the front door for squad routing (vimy-b14).
// ApproachWaypoint remains available as a lower-level utility.
func (e RuleEnv) BestApproachAxis(destX, destY int) (int, int, bool) {
	return e.bestApproachAxisWithField(destX, destY, e.ThreatField())
}

// BestAirApproachAxis is the AA-only counterpart of BestApproachAxis.
func (e RuleEnv) BestAirApproachAxis(destX, destY int) (int, int, bool) {
	return e.bestApproachAxisWithField(destX, destY, e.AirThreatField())
}

func (e RuleEnv) bestApproachAxisWithField(destX, destY int, field *model.ThreatField) (int, int, bool) {
	if e.Terrain == nil || e.Terrain.CellW <= 0 || e.Terrain.CellH <= 0 || field == nil {
		return 0, 0, false
	}
	centX, centY := e.BuildingCentroid()

	directScore := corridorThreatSum(field, e.Terrain, centX, centY, destX, destY)
	if directScore < openEnoughThreshold {
		return 0, 0, false
	}

	radiusMap := approachAxisRadiusCells * maxInt(e.Terrain.CellW, e.Terrain.CellH)

	bestScore := math.Inf(1)
	var bestX, bestY int
	for i := 0; i < 8; i++ {
		angle := float64(i) * math.Pi / 4
		wx := destX + int(float64(radiusMap)*math.Cos(angle))
		wy := destY + int(float64(radiusMap)*math.Sin(angle))
		wx = clampInt(wx, 0, e.State.MapWidth-1)
		wy = clampInt(wy, 0, e.State.MapHeight-1)
		score := corridorThreatSum(field, e.Terrain, centX, centY, wx, wy)
		if score < bestScore {
			bestScore = score
			bestX = wx
			bestY = wy
		}
	}

	// Only detour if the best flank is meaningfully cleaner than direct. The
	// 0.8 ratio avoids constant re-routing when all candidates score similar.
	if bestScore >= directScore*0.8 {
		return 0, 0, false
	}
	return bestX, bestY, true
}

// corridorThreatSum sums the threat field cells inside the axis-aligned
// bounding box between two map positions. Same primitive used by the older
// ApproachWaypoint gate.
func corridorThreatSum(field *model.ThreatField, t *model.TerrainGrid, ax, ay, bx, by int) float64 {
	if t == nil || t.CellW <= 0 || t.CellH <= 0 {
		return 0
	}
	aCol, aRow := ax/t.CellW, ay/t.CellH
	bCol, bRow := bx/t.CellW, by/t.CellH
	minCol, maxCol := aCol, bCol
	if minCol > maxCol {
		minCol, maxCol = maxCol, minCol
	}
	minRow, maxRow := aRow, bRow
	if minRow > maxRow {
		minRow, maxRow = maxRow, minRow
	}
	sum := 0.0
	for r := minRow; r <= maxRow; r++ {
		for c := minCol; c <= maxCol; c++ {
			sum += field.At(c, r)
		}
	}
	return sum
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (e RuleEnv) approachWaypointWithField(destX, destY int, field *model.ThreatField) (int, int, bool) {
	if e.Terrain == nil || e.Terrain.CellW <= 0 || e.Terrain.CellH <= 0 {
		return 0, 0, false
	}
	centX, centY := e.BuildingCentroid()
	from := [2]int{centX / e.Terrain.CellW, centY / e.Terrain.CellH}
	to := [2]int{destX / e.Terrain.CellW, destY / e.Terrain.CellH}
	if field == nil {
		return 0, 0, false
	}

	const penalty = 5.0
	const bboxHeatThreshold = 1.0 // cumulative threat in the corridor between base and goal
	const hotThreshold = 0.4      // per-zone threat that marks a zone as contested

	// Gate: sum threat inside the axis-aligned bounding box between base and
	// goal. BFS tie-breaking makes "direct path heat" unreliable, but a fat
	// bounding-box scan catches defense placements that sit between us and
	// the target regardless of which shortest path BFS happens to pick.
	minCol, maxCol := from[0], to[0]
	if minCol > maxCol {
		minCol, maxCol = maxCol, minCol
	}
	minRow, maxRow := from[1], to[1]
	if minRow > maxRow {
		minRow, maxRow = maxRow, minRow
	}
	bboxHeat := 0.0
	for row := minRow; row <= maxRow; row++ {
		for col := minCol; col <= maxCol; col++ {
			bboxHeat += field.At(col, row)
		}
	}
	if bboxHeat < bboxHeatThreshold {
		return 0, 0, false
	}

	// Find the weighted path and pick a staging zone on it. Preference order:
	//   1. Last cool zone before the path first enters a hot cell.
	//   2. Second-to-last path zone (the squad forms up just before the goal
	//      along the alternate approach the weighted BFS picked).
	weighted := model.ApproachPath(e.Terrain, field, from, to, penalty)
	if len(weighted) < 2 {
		return 0, 0, false
	}

	var wp [2]int
	found := false
	for i, p := range weighted {
		if i == 0 {
			continue
		}
		if field.At(p[0], p[1]) >= hotThreshold {
			wp = weighted[i-1]
			found = true
			break
		}
	}
	if !found {
		wp = weighted[len(weighted)-2]
		found = true
	}
	if wp == from {
		return 0, 0, false
	}
	x, y := e.Terrain.ZoneCenter(wp[0], wp[1])
	return x, y, true
}

// CriticalBuildingUnderAttack reports whether any of our critical
// infrastructure (CY, WF, refinery, power plants, barracks, tech center) is
// currently taking damage or has an enemy within close-range attack distance.
// Used by the defend-critical-building rule to override normal squad-pool
// filtering — when the CY is being shot at by rocket launchers, ALL nearby
// ground units should engage, including committed ground-attack squad
// members that would otherwise be excluded by scramble/emergency defense
// (vimy-d9q's poach-prevention). Game 59 lost the CY to 2 rocket launchers
// while an idle mammoth tank sat in the ground-attack squad, never poached
// because scramble excludes squad members and emergency needs zero idle.
func (e RuleEnv) CriticalBuildingUnderAttack() bool {
	return e.nearestEnemyAttackingCritical() != nil
}

// nearestEnemyAttackingCritical returns the nearest enemy that's within
// attack range of one of our critical buildings, or nil if no critical
// building is under threat. Attack range is 10 map cells — tight enough
// to exclude passing scouts, wide enough to catch attackers about to
// engage.
func (e RuleEnv) nearestEnemyAttackingCritical() *model.Enemy {
	const attackRange = 10
	attackRangeSq := attackRange * attackRange

	var nearest *model.Enemy
	var nearestDistSq int = 1<<31 - 1
	for i := range e.State.Buildings {
		b := &e.State.Buildings[i]
		if !isCriticalRepairType(b.Type) {
			continue
		}
		for j := range e.State.Enemies {
			en := &e.State.Enemies[j]
			// Skip harvesters, non-combat enemies, and husk wreckage.
			// Husks persist briefly in State.Enemies after a unit dies
			// (game 60: defend-critical-building fired 191 times mostly
			// targeting heli.husk after the helicopter was already killed).
			if matchesType(en.Type, Harvester) || matchesType(en.Type, MCV) || isHuskType(en.Type) {
				continue
			}
			dx := en.X - b.X
			dy := en.Y - b.Y
			distSq := dx*dx + dy*dy
			if distSq > attackRangeSq {
				continue
			}
			if distSq < nearestDistSq {
				nearestDistSq = distSq
				nearest = en
			}
		}
	}
	return nearest
}

// isHuskType reports whether the type name is a wreckage/husk artifact
// (e.g., "heli.husk"). OpenRA keeps husks in the actor list for a short
// window after death; they're not attackable in a meaningful sense and
// should not drive defensive engagement.
func isHuskType(t string) bool {
	return strings.HasSuffix(strings.ToLower(t), ".husk")
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

// CanBuildAnyCombatVehicle reports whether the generic production rule has
// anything worth building.
//
// Excludes the roles with their own capped rules, so that when a flak truck is
// the only option `produce-vehicle` holds rather than firing and picking one —
// which is the whole failure in vimy-77s. Holding is the right answer: the
// dedicated rule still builds them up to its cap, and the cash stays available
// for the radar that would unlock a real combat vehicle.
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

// BestBuildableVehicle returns the highest-priority buildable combat vehicle.
// Checks LLM preferences first, then falls back to combatVehicleRoles.
// Uses round-robin fairness so preferred roles don't starve others.
func (e RuleEnv) BestBuildableVehicle() string {
	// The preference list is filtered too, not just the fallback. In the game
	// that reported this the doctrine listed flak_truck third, so walking
	// preferences found heavy and medium tanks unbuildable and landed on it
	// every time — the fallback was never even reached.
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

// BestBuildableSpecialist picks the best available elite infantry.
// Checks LLM preferences first, then falls back to specialistInfantryRoles.
// Only considers specialist roles from preferences — non-specialists like
// engineer and rocket_soldier are skipped so they don't hijack the slot.
// Uses round-robin fairness so preferred roles don't starve others.
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

// bestBuildableFrom picks the buildable role from candidates with the fewest
// existing units. Among roles tied at the minimum count, the first in the list
// wins (preserving preference order). This cycles production across roles
// while still respecting priority. If allowSet is non-nil, only roles in the
// set are considered (used to filter non-specialist infantry from preferences).
func (e RuleEnv) bestBuildableFrom(candidates []string, allowSet map[string]bool) string {
	// Find the minimum count among buildable candidates.
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
	// Pick the first candidate at the minimum count (preference order tiebreak).
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

// BestBuildableAircraft picks the best available combat aircraft.
// Checks LLM preferences first, then falls back to combatAircraftRoles.
// Uses round-robin fairness so preferred roles don't starve others.
func (e RuleEnv) BestBuildableAircraft() string {
	if item := e.bestBuildableFrom(e.Preferences.Aircraft, nil); item != "" {
		return item
	}
	return e.bestBuildableFrom(combatAircraftRoles, nil)
}

// BestBuildableNaval picks the best available naval combat unit.
// Checks LLM preferences first, then falls back to combatNavalRoles.
// Uses round-robin fairness so preferred roles don't starve others.
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
