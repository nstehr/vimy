package rules

import (
	"log/slog"
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
	v, _ := e.Memory["beingRushed"].(bool)
	return v
}

// IsHarvesterHarassed reports sustained mid-game harvester pressure, which
// rules use to tell a raid apart from a rush.
func (e RuleEnv) IsHarvesterHarassed() bool {
	v, _ := e.Memory["harvesterHarassed"].(bool)
	return v
}

// SquadClumped reports whether ≥80% of living members are within radiusCells of
// the centroid. Gates squad-attack on the squad having actually assembled: a
// strung-out squad is picked off one unit at a time.
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
	return near*10 >= len(members)*8
}

// AxisBurned reports an axis the doctrine has repeatedly pivoted to and been
// countered on. A hard gate rather than a prompt constraint: told only in the
// prompt, the LLM kept committing aircraft into flak.
func (e RuleEnv) AxisBurned(axis string) bool {
	m, ok := e.Memory["burnedAxes"].(map[string]bool)
	if !ok {
		return false
	}
	return m[axis]
}

// OverextendedSquadMembers returns idle squad members beyond leashPct of the
// map diagonal that are also not making forward progress — closer to our base
// than to any known enemy base.
//
// The progress test is what keeps recall from looping: units staging at a flank
// waypoint, in transit, or fighting at the enemy base all sit far from home and
// arrive idle. Recalled, they re-form, get sent out again, and arrive idle again.
// A fixed radius around the enemy base was too narrow to cover flank staging.
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
		// Closer to the enemy than to home means in transit or staging.
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

// SquadThreatRatio is local enemy HP over squad HP; above 1.0 we are outmatched
// where we stand. radiusPct is a fraction of the map diagonal.
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
		// Unarmed structures are the squad's OBJECTIVE, not its danger. The mod
		// includes buildings in the enemy list on purpose, so a squad sent to
		// attack a base counted the base as the force opposing it: game 94 read
		// a median threat ratio of 7.96 and a p90 of 21 while engaged, and
		// squad-disengage — which fires above about 2.5 — decided to withdraw
		// 31 times against 15 attacks. It was retreating from what it came to
		// destroy. Defensive structures still count; a pillbox is a real reason
		// to leave.
		if IsUnarmedStructure(en.Type) {
			continue
		}
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

// WeakestVisibleEnemy returns the lowest HP/MaxHP ratio, ties broken by
// proximity.
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

// HarvesterThreatRatio is the enemy strength, relative to a harvester's own
// health, that counts as danger.
//
// Presence alone doesn't: a lone rifleman is otherwise indistinguishable from a
// tank column, and it parks the economy shuttling between ore and refinery.
//
// Relative rather than absolute HP, matching SquadThreatRatio, so it needs no
// table of unit health and doesn't drift when the mod retunes one.
const HarvesterThreatRatio = 0.5

// HarvestersInDanger returns harvesters, idle or not, facing enough nearby
// enemy strength to be worth running from. dangerPct is a fraction of the map
// diagonal.
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
		// Full health, not current: a damaged harvester shouldn't get easier to
		// spook as it takes more damage.
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

func (e RuleEnv) CapturableCount() int { return len(e.State.Capturables) }

// capturableValue ranks capturable types; unknown types get a baseline score.
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

// BestCapturable divides value by sqrt(distance), so a nearby cheap target can
// beat a distant valuable one whose trip isn't worth making.
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

// EngineerNearCapturable catches an engineer just unloaded next to its target.
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

// airTargetValue ranks air-strike targets. Defenses score high because aircraft
// can reach them ahead of a ground push that otherwise has to eat them.
var airTargetValue = map[string]float64{
	// Win-condition target — same reasoning as groundTargetValue.
	ConstructionYard: 12, "afac": 12,
	// Defense structures — clear them so we can stay over the base.
	TeslaCoil: 10, Turret: 8, Pillbox: 7, CamoPillbox: 7, FlameTower: 6,
	// Superweapons
	MissileSilo: 9, IronCurtain: 9,
	// Production
	WarFactory: 5, Airfield: 5, Helipad: 5,
	SovietBarracks: 4, AlliedBarracks: 4,
	// Economy
	Refinery: 6,
	// AA defenses (risky but worth removing)
	AAGun: 4, SAMSite: 4,
	// Power / support
	AdvancedPower: 3, PowerPlant: 2, RadarDome: 3, AlliedTechCenter: 3, SovietTechCenter: 3,
	SubPen: 4, NavalYard: 4,
}

const airTargetValueDefault = 1.0 // mobile units / unknown types

// BestAirTarget scores val * hpBonus / sqrt(dist): type value dominates, damage
// breaks ties, distance decays gently.
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
		base := strings.ToLower(en.Type)
		if idx := strings.IndexByte(base, '.'); idx >= 0 {
			base = base[:idx] // faction suffix: "afld.ukraine" → "afld"
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

// groundTargetValue ranks ground-attack targets. Active base defenses score
// high because they are killing the push; AA and naval score low because they
// can't touch it.
var groundTargetValue = map[string]float64{
	// Leads the table: destroying the CY is the actual win condition, and
	// squads otherwise chase mobile units instead of pressing infrastructure.
	ConstructionYard: 12,
	// Active base defenses — kill these first so the push survives.
	TeslaCoil: 10, Turret: 8, Pillbox: 7, CamoPillbox: 7, FlameTower: 6,
	// Superweapons
	MissileSilo: 9, IronCurtain: 9,
	// AA defenses (low threat to ground)
	AAGun: 2, SAMSite: 2,
	// Production (destroy their ability to replace losses)
	WarFactory: 6, Airfield: 5, Helipad: 5, SovietBarracks: 4, AlliedBarracks: 4,
	// Economy — strangulation is a real win path once defenses are down.
	Refinery: 7,
	// Naval (low priority for ground forces)
	SubPen: 2, NavalYard: 2,
	// Power / support
	AdvancedPower: 3, PowerPlant: 2, RadarDome: 3, AlliedTechCenter: 3, SovietTechCenter: 3,
}

const groundTargetValueDefault = 1.0 // mobile units / unknown types

// BestGroundTarget scores val * hpBonus / (1 + dist/groundDistanceScale). The
// decay is soft on purpose: under 1/dist, a value-1 unit at the squad's feet
// always beat a war factory across the map, and squads chased trash forever.
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
		base := strings.ToLower(en.Type)
		if idx := strings.IndexByte(base, '.'); idx >= 0 {
			base = base[:idx] // faction suffix: "afld.ukraine" → "afld"
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

// SquadReadyRatio is the idle fraction of *available* members. Retreating units
// leave both numerator and denominator, so a doctrine demanding 100% readiness
// isn't blocked forever by one unit healing at the depot.
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

// SquadAwayFromBase keeps disengage from firing on a squad already at home.
// radiusPct is a fraction of the map diagonal.
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

// EnemyBaseIntel records a known enemy base position.
type EnemyBaseIntel struct {
	Owner         string
	X             int
	Y             int
	Tick          int
	FromBuildings bool // true if derived from building sightings (high confidence)
}

// enemyHarvesterIntel persists last-known enemy harvester positions. Harvesters
// orbit refineries inside the enemy base, so a stale sighting still locates it.
type enemyHarvesterIntel struct {
	X, Y, Tick int
}

// IntelCenter estimates the enemy base from all accumulated sightings.
// Confidence scales 0..1 with weighted sighting mass — one harvester is weak
// evidence, several buildings strong.
type IntelCenter struct {
	X, Y       int
	Confidence float64
	Sources    int
}

// knownBuildingTypes separates structures from mobile units in the Enemies
// list: a building fixes a base, a unit may just be a force passing through.
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

// updateIntel maintains known enemy base positions. Buildings always update;
// units only seed, so a roaming attack force can't overwrite a real base.
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

	for owner, a := range buildingsByOwner {
		bases[owner] = EnemyBaseIntel{
			Owner:         owner,
			X:             a.sumX / a.count,
			Y:             a.sumY / a.count,
			Tick:          env.State.Tick,
			FromBuildings: true,
		}
	}

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

	// Forget a base only after our units have stood near it for a sustained
	// stretch without seeing anything. A single-tick check lets a scout passing
	// through wipe good intel the moment the enemy sits behind fog, and the
	// attack rules downstream then have no target at all.
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

	harvesters := memoryMap[int, enemyHarvesterIntel](env.Memory, "enemyHarvesterIntel")
	for _, e := range env.State.Enemies {
		if matchesType(e.Type, Harvester) {
			harvesters[e.ID] = enemyHarvesterIntel{X: e.X, Y: e.Y, Tick: env.State.Tick}
		}
	}

	updateDefenseIntel(env)

	// Units dedupe by ID; buildings track a high-water mark instead, since they
	// are destroyed and rebuilt under fresh IDs.
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

// HasEnemyIntel requires a building sighting — scouting continues until we
// find the base itself, not just its patrols.
func (e RuleEnv) HasEnemyIntel() bool {
	for _, base := range getEnemyBases(e.Memory) {
		if base.FromBuildings {
			return true
		}
	}
	return false
}

// InferredEnemyBaseCenter is the weighted centroid of every enemy sighting we
// hold. The persistent base centroid weighs most (it already aggregates many
// building sightings), then visible buildings, then harvesters, which decay
// with age. Lets siege and rally rules aim at a base seen only in fragments.
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

// EnemyDefenseIntel outlives vision so path planning can route around defenses
// currently behind fog.
type EnemyDefenseIntel struct {
	ActorID int
	Type    string
	X, Y    int
	Tick    int
}

// defenseThreatWeight is painted onto the threat field, ordered by rough
// DPS × range.
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

// updateDefenseIntel refreshes remembered defenses. Stale records clear only
// when one of our units stands where the defense should be and doesn't see it —
// same evidence standard as base intel.
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

// ThreatField rasterizes remembered defenses onto a field parallel to the
// terrain grid.
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

// airDefenseThreatWeight covers only what actually shoots aircraft. Weighted
// above the ground equivalents: a strike package can't trade with a SAM cluster
// the way a ground push can trade with a pillbox.
var airDefenseThreatWeight = map[string]float64{
	AAGun:   2.0,
	SAMSite: 2.5,
}

// AirThreatField is the AA-only counterpart of ThreatField.
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

// ApproachWaypoint is a staging point short of dest: the last low-threat zone
// on the weighted path, where a squad forms up before committing. False when a
// detour buys nothing — open approach, no intel, unreachable.
func (e RuleEnv) ApproachWaypoint(destX, destY int) (int, int, bool) {
	return e.approachWaypointWithField(destX, destY, e.ThreatField())
}

// AirApproachWaypoint mirrors ApproachWaypoint against AA defenses only.
func (e RuleEnv) AirApproachWaypoint(destX, destY int) (int, int, bool) {
	return e.approachWaypointWithField(destX, destY, e.AirThreatField())
}

// approachAxisRadiusCells places flank candidates far enough out to escape a
// defense cluster's painted radius — otherwise every candidate scores hot —
// while staying a small detour relative to the base-to-target distance.
const approachAxisRadiusCells = 8

// openEnoughThreshold is where a corridor counts as already open and the whole
// detour mechanism switches off. Roughly the signal of 1-2 distant defenses.
const openEnoughThreshold = 1.0

// BestApproachAxis returns the compass flank around dest whose corridor from
// base carries the least threat. Comparing eight candidates finds the open side
// of a lopsidedly defended base, which ApproachWaypoint's single weighted BFS
// path does not reliably do — hence this, not that, is the front door for squad
// routing. False when data is missing, the direct corridor is already open, or
// no flank is meaningfully cleaner.
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

	// A margin, not a strict win: near-tied candidates would re-route constantly.
	if bestScore >= directScore*0.8 {
		return 0, 0, false
	}
	return bestX, bestY, true
}

// corridorThreatSum sums threat field cells in the bounding box between two
// map positions.
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

	// A bounding-box scan rather than the heat along the direct path: which
	// shortest path BFS returns is a tie-break artifact, and defenses between
	// us and the target matter whichever one it picks.
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

	// Stage at the last cool zone before the path turns hot; failing that, just
	// short of the goal on whatever approach the weighted BFS chose.
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

// CriticalBuildingUnderAttack reports whether core infrastructure is taking
// damage or has an enemy in range. It is the gate that lets defend-critical-
// building ignore squad reservations: without it, a mammoth tank sits idle in
// the attack squad while rocket launchers take down the CY.
func (e RuleEnv) CriticalBuildingUnderAttack() bool {
	return e.nearestEnemyAttackingCritical() != nil
}

// nearestEnemyAttackingCritical returns the nearest enemy in range of critical
// infrastructure. 10 cells excludes passing scouts but catches an attacker
// about to engage.
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
			// Husks linger in State.Enemies after death and will otherwise draw
			// the whole defense onto an already-dead helicopter.
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

// isHuskType matches the wreckage OpenRA leaves in the actor list briefly after
// a death — not a meaningful target.
func isHuskType(t string) bool {
	return strings.HasSuffix(strings.ToLower(t), ".husk")
}

// BaseUnderAttack triggers within 20% of the map diagonal — far enough out to
// catch an attack before it lands, close enough to ignore distant enemies.
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
