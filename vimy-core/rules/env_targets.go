package rules

import (
	"math"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
)

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

const groundTargetValueDefault = 1.0

// BestGroundTarget scores val * hpBonus / (1 + dist/groundDistanceScale). The
// decay is soft on purpose: under 1/dist, a value-1 unit at the squad's feet
// always beat a war factory across the map, and squads chased trash forever.
const groundDistanceScale = 50.0

// BestGroundTarget scores targets from our own base, which is what defending
// wants: the thing nearest home is the thing to kill.
func (e RuleEnv) BestGroundTarget() *model.Enemy {
	bx, by := 0, 0
	if len(e.State.Buildings) > 0 {
		bx = e.State.Buildings[0].X
		by = e.State.Buildings[0].Y
	}
	return e.BestGroundTargetFrom(bx, by)
}

// BestGroundTargetFrom scores from wherever the fighting is.
//
// Distance was always measured from our own base, for attacking as well as
// defending, so a squad standing inside the enemy base scored the construction
// yard beside it as maximally distant while a rifleman back home scored at
// nearly full value. The table has the construction yard at 12 and a mobile
// unit at 1, and the divisor undid that: five measured games destroyed 0, 0, 0,
// 1 and 0 enemy buildings while killing 184750 credits of enemy units in the
// last of them.
// NearestRememberedStructure is the closest enemy building whose position is
// known, whether or not anything can currently see it.
//
// Sight is fleeting and memory is not. Without this the assault had nothing to
// aim at once a building went back into fog, so it spiralled outward from the
// base centroid on huntOffset and recorded blind-at-base when it arrived and
// saw nothing - 37 times in game 173, with the squad standing on the enemy
// base. Scored by distance alone: the value table needs a live actor, and the
// question here is only "where is there something to break".
//
// Returns a synthetic Enemy so callers that already take a target need no
// special case. The zero ActorID marks it as remembered rather than sighted,
// which matters because an attack order needs a real actor - a caller that
// wants to SHOOT must find the thing when it arrives, not fire at a memory.
func (e RuleEnv) NearestRememberedStructure(fromX, fromY int) *model.Enemy {
	best := math.MaxFloat64
	var out *model.Enemy
	for _, st := range getEnemyStructures(e.Memory) {
		dx, dy := float64(st.X-fromX), float64(st.Y-fromY)
		if d := dx*dx + dy*dy; d < best {
			best = d
			out = &model.Enemy{ID: st.ActorID, Type: st.Type, X: st.X, Y: st.Y}
		}
	}
	return out
}

func (e RuleEnv) BestGroundTargetFrom(bx, by int) *model.Enemy {
	return e.bestGroundTargetFrom(bx, by, false)
}

// BestGroundStructureFrom is the same ranking restricted to buildings.
//
// The strike wants a structure, and asking the unrestricted ranking for one and
// giving up when a defender outscored it was worth 371 abandoned strikes in game
// 180 - 10.7 per thousand ticks against 0.18 in game 178 - in a game that
// destroyed one enemy building in 34790 ticks. Same scores, so the building this
// returns is the one the unrestricted call would have named had the units not
// been standing in front of it.
func (e RuleEnv) BestGroundStructureFrom(bx, by int) *model.Enemy {
	return e.bestGroundTargetFrom(bx, by, true)
}

func (e RuleEnv) bestGroundTargetFrom(bx, by int, buildingsOnly bool) *model.Enemy {
	if len(e.State.Enemies) == 0 {
		return nil
	}
	var best *model.Enemy
	bestScore := -1.0
	for i := range e.State.Enemies {
		en := &e.State.Enemies[i]
		if en.MaxHP == 0 {
			continue
		}
		if buildingsOnly && !IsKnownBuildingType(en.Type) {
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

// CriticalBuildingUnderAttack reports whether core infrastructure has been hit
// and the attacker is still in range. It is the gate that lets defend-critical-
// building ignore squad reservations: without it, a mammoth tank sits idle in
// the attack squad while rocket launchers take down the CY.
//
// Both halves are required, and the damage half used to be missing — the doc
// claimed "taking damage or has an enemy in range" and the code only checked
// range. Every enemy that drove within 10 cells of the construction yard
// conscripted the entire army, which under raid pressure is permanent: game
// 137 acted on this 536 times in 48460 ticks, once every 90, and it replaced
// emergency-base-defense as the thing dragging the assault home the moment
// that rule stopped poaching.
//
// Damage rather than a damage DELTA, because a delta needs a previous state
// this has no access to. "Has been hit and they are still standing over it" is
// the honest approximation: an undamaged building with an enemy passing is not
// an emergency, and a scarred building with nobody near it is not either. The
// cost is that the first shot must land before the whole army reacts. For a
// rule that overrides every squad reservation in the set, reacting to hits
// rather than to proximity is the right posture — the garrison answers
// proximity, via emergency-base-defense.
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
		// Unhit infrastructure is not an emergency, whatever is driving past.
		if b.MaxHP <= 0 || b.HP >= b.MaxHP {
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
// baseBuildings is what counts as "the base" for proximity: everything we own
// except captured neutral tech structures.
//
// An oil derrick is a building we own and is nowhere near the base. Game 193
// held three, at 28, 45 and 76 cells from the construction yard, and the
// proximity threshold is 20 percent of the map diagonal -- about 26 cells -- so
// each one added its own "the base is under attack" bubble in a far corner and
// the union covered most of the map. 52 percent of defend-base's 606 firings in
// that game had the nearest enemy next to a DERRICK and none within 26 cells of
// the real base, which recalled the army from the enemy's doorstep to defend an
// oil well.
//
// Invisible until capture started working: before the capture gate was fixed,
// Vimy took two derricks in ten sessions, and it now takes three a game.
func (e RuleEnv) baseBuildings() []model.Building {
	out := make([]model.Building, 0, len(e.State.Buildings))
	for _, b := range e.State.Buildings {
		if IsNeutralTechStructure(b.Type) {
			continue
		}
		out = append(out, b)
	}
	return out
}

func (e RuleEnv) BaseUnderAttack() bool {
	base := e.baseBuildings()
	if len(base) == 0 || len(e.State.Enemies) == 0 {
		return false
	}
	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	threshold := math.Sqrt(mw*mw+mh*mh) * 0.20
	threshSq := threshold * threshold

	for i := range e.State.Enemies {
		for j := range base {
			dx := float64(e.State.Enemies[i].X - base[j].X)
			dy := float64(e.State.Enemies[i].Y - base[j].Y)
			if dx*dx+dy*dy < threshSq {
				return true
			}
		}
	}
	return false
}
