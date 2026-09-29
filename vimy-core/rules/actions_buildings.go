package rules

import (
	"log/slog"
	"math"
	"math/rand"
	"slices"
	"strings"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

// mcvDeployState detects an MCV whose Deploy the engine keeps silently
// rejecting (usually no clear 3x3 footprint) so we relocate instead of
// resending the same failing order forever.
type mcvDeployState struct {
	Attempts int
	Tick     int
	// Relocation count, so successive fallbacks pick a different tile rather
	// than re-deriving the same one.
	Relocations int
}

const (
	mcvDeployCooldownTicks = 50
	mcvMaxDeployAttempts   = 3
	// How long a harvester is left to the engine's own ore search before the
	// rule steps in. Long enough that a normal unload-and-return cycle is never
	// interrupted.
	harvesterIdleGrace = 250
	guardResendTicks   = 100
	// How long a squad that has withdrawn is left alone by the attacking rules.
	// squad-reengage sends any idle squad member at the enemy the moment it
	// stops moving, so without this a withdrawal walks home, arrives, goes idle
	// and is immediately sent back — which in game 95 was the busiest combat
	// rule in the game at 32 acts against 8 deliberate attacks. Distinct from
	// the damaged-unit mark, which clear-healed-units releases as soon as the
	// unit is at full health: a unit that withdrew intact is exactly the case
	// that would release instantly.
	disengageHoldTicks   = 300
	disengageResendTicks = 100
	mcvFallbackStep      = 250
	mcvFallbackMaxRings  = 3
)

func ActionDeployMCV(env RuleEnv, conn CommandSender) error {
	if lastTick, ok := env.Memory["deployMCVTick"].(int); ok {
		if env.State.Tick-lastTick < mcvDeployCooldownTicks {
			return nil
		}
	}

	var target *model.Unit
	for i := range env.State.Units {
		u := &env.State.Units[i]
		if matchesType(u.Type, MCV) && u.Idle {
			target = u
			break
		}
	}
	if target == nil {
		return nil
	}

	states := memoryMap[int, mcvDeployState](env.Memory, "mcvDeployState")
	state := states[target.ID]
	state.Tick = env.State.Tick

	// Repeated failures mean the spot is unusable, not the order — move first,
	// retry Deploy when it next arrives idle.
	if state.Attempts >= mcvMaxDeployAttempts {
		fx, fy := mcvFallbackLocation(env, target, state.Relocations)
		state.Attempts = 0
		state.Relocations++
		states[target.ID] = state
		env.Memory["deployMCVTick"] = env.State.Tick
		slog.Info("relocating stuck MCV", "id", target.ID, "to_x", fx, "to_y", fy)
		return conn.Send(ipc.TypeMove, ipc.MoveCommand{
			ActorID: uint32(target.ID),
			X:       fx,
			Y:       fy,
		})
	}

	state.Attempts++
	states[target.ID] = state
	env.Memory["deployMCVTick"] = env.State.Tick
	slog.Debug("deploying MCV", "id", target.ID, "attempt", state.Attempts)
	return conn.Send(ipc.TypeDeploy, ipc.DeployCommand{
		ActorID: uint32(target.ID),
	})
}

// mcvFallbackLocation picks a land tile for a retry deploy.
//
// Anchored on the MCV, not the building centroid: the centroid follows what is
// still standing, so during an overrun it converges on the fighting — the worst
// place to deploy, and the only situation this runs in.
//
// round rotates the direction and widens the ring so a failed spot isn't the
// first thing tried again.
func mcvFallbackLocation(env RuleEnv, mcv *model.Unit, round int) (int, int) {
	cx, cy := mcv.X, mcv.Y
	if env.Terrain == nil {
		return cx, cy
	}
	dirs := [8][2]int{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},
		{1, 1}, {-1, 1}, {1, -1}, {-1, -1},
	}
	// Capped so a long game can't walk the MCV into an unreachable corner.
	dist := mcvFallbackStep * (1 + min(round, mcvFallbackMaxRings))
	for i := range dirs {
		o := dirs[(i+round)%len(dirs)]
		tx := clampInt(cx+o[0]*dist, 0, env.State.MapWidth-1)
		ty := clampInt(cy+o[1]*dist, 0, env.State.MapHeight-1)
		if env.Terrain.AtMapPos(tx, ty) == model.Land {
			return tx, ty
		}
	}
	return cx, cy
}

func ActionPlaceDefense(env RuleEnv, conn CommandSender) error {
	for _, pq := range env.State.ProductionQueues {
		if strings.EqualFold(pq.Type, QueueDefense) && pq.CurrentItem != "" && pq.CurrentProgress >= 100 {
			hx, hy := defenseHint(env)
			slog.Debug("placing defense", "item", pq.CurrentItem, "hint_x", hx, "hint_y", hy)
			return conn.Send(ipc.TypePlaceBuilding, ipc.PlaceBuildingCommand{
				Queue: QueueDefense,
				Item:  pq.CurrentItem,
				HintX: hx,
				HintY: hy,
			})
		}
	}
	return nil
}

// chokePos pre-computes a ranked chokepoint's map-space center so candidate
// scoring doesn't re-walk the grid.
type chokePos struct {
	x, y  int
	score float64
}

// defenseHint scores candidates around the base perimeter and picks randomly
// from the top 3 — deterministic placement is trivially exploitable.
func defenseHint(env RuleEnv) (int, int) {
	// The real base: defences placed around a captured derrick's share of the
	// centroid defend an oil well nobody is attacking.
	buildings := env.baseBuildings()
	if len(buildings) == 0 {
		buildings = env.State.Buildings
	}
	if len(buildings) == 0 {
		return 0, 0
	}

	sumX, sumY := 0, 0
	for _, b := range buildings {
		sumX += b.X
		sumY += b.Y
	}
	cx := sumX / len(buildings)
	cy := sumY / len(buildings)

	var maxDistSq float64
	for _, b := range buildings {
		dx := float64(b.X - cx)
		dy := float64(b.Y - cy)
		if d := dx*dx + dy*dy; d > maxDistSq {
			maxDistSq = d
		}
	}
	radius := math.Sqrt(maxDistSq)
	if radius < 3 {
		radius = 3
	}

	var threatX, threatY float64
	hasThreat := false
	if base := env.NearestEnemyBase(); base != nil {
		dx := float64(base.X - cx)
		dy := float64(base.Y - cy)
		d := math.Sqrt(dx*dx + dy*dy)
		if d > 0 {
			threatX = dx / d
			threatY = dy / d
			hasThreat = true
		}
	}

	highValueTypes := []string{
		ConstructionYard, Refinery, WarFactory,
		AlliedTechCenter, SovietTechCenter,
		MissileSilo, IronCurtain, Airfield, Helipad,
	}
	var hvBuildings []model.Building
	for _, b := range buildings {
		for _, t := range highValueTypes {
			if matchesType(b.Type, t) {
				hvBuildings = append(hvBuildings, b)
				break
			}
		}
	}

	defenseTypes := []string{Pillbox, CamoPillbox, Turret, FlameTower, TeslaCoil, AAGun, SAMSite}
	var defenses []model.Building
	for _, b := range buildings {
		for _, t := range defenseTypes {
			if matchesType(b.Type, t) {
				defenses = append(defenses, b)
				break
			}
		}
	}

	// Chokepoints seed candidates directly, so one adjacent to base can win
	// placement outright rather than only nudging annulus scores.
	//
	// Fleeing harvesters mean the threat is on the economy, not the front, so
	// refineries seed their own candidates and outweigh the enemy-facing axis.
	harvesterEmergency := CountFleeingHarvesters(env.Memory) >= 2
	var refineries []model.Building
	for _, b := range buildings {
		if matchesType(b.Type, Refinery) {
			refineries = append(refineries, b)
		}
	}

	chokes := env.ChokepointsTowardEnemy()
	var nearbyChokes []chokePos
	if env.Terrain != nil {
		maxChokeDist := 2.0 * radius
		for _, c := range chokes {
			zx, zy := env.Terrain.ZoneCenter(c.Col, c.Row)
			dx := float64(zx - cx)
			dy := float64(zy - cy)
			d := math.Sqrt(dx*dx + dy*dy)
			if d > maxChokeDist {
				continue
			}
			// Flank the choke rather than sit on it — a pillbox on the only
			// bridge tile strands our own harvesters and reinforcements.
			sx, sy := zx, zy
			if d > 0 {
				offset := 0.5 * float64(env.Terrain.CellW)
				if env.Terrain.CellH < env.Terrain.CellW {
					offset = 0.5 * float64(env.Terrain.CellH)
				}
				sx = zx - int(dx/d*offset)
				sy = zy - int(dy/d*offset)
			}
			// Better to skip the seed than block a crossing.
			if t := env.Terrain.AtMapPos(sx, sy); t != model.Land {
				continue
			}
			nearbyChokes = append(nearbyChokes, chokePos{x: sx, y: sy, score: c.Score})
		}
	}

	type candidate struct {
		x, y  int
		score float64
		seed  bool // true if originated from a choke seed (for logging)
	}
	var candidates []candidate
	var sampleXs, sampleYs []int
	var sampleSeed []bool
	for i := range 16 {
		angle := float64(i) * 2 * math.Pi / 16
		r := radius * (1.0 + rand.Float64()*0.5)
		sampleXs = append(sampleXs, cx+int(r*math.Cos(angle)))
		sampleYs = append(sampleYs, cy+int(r*math.Sin(angle)))
		sampleSeed = append(sampleSeed, false)
	}
	for _, c := range nearbyChokes {
		sampleXs = append(sampleXs, c.x)
		sampleYs = append(sampleYs, c.y)
		sampleSeed = append(sampleSeed, true)
	}
	// A cardinal ring per refinery covers the common raid approach angles.
	if harvesterEmergency && len(refineries) > 0 {
		const refineryRingRadius = 3.0
		offset := 96.0 // ~3 cells at typical 32-px cell width
		if env.Terrain != nil {
			offset = refineryRingRadius * float64(maxInt(1, env.Terrain.CellW))
		}
		dirs := [4][2]float64{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
		for _, r := range refineries {
			for _, d := range dirs {
				sampleXs = append(sampleXs, r.X+int(offset*d[0]))
				sampleYs = append(sampleYs, r.Y+int(offset*d[1]))
				sampleSeed = append(sampleSeed, true)
			}
		}
	}
	for i := range sampleXs {
		x := sampleXs[i]
		y := sampleYs[i]
		seed := sampleSeed[i]

		// Bridge is excluded along with water/cliff: buildable, but it would
		// block our own units crossing.
		if env.Terrain != nil {
			if env.Terrain.AtMapPos(x, y) != model.Land {
				continue
			}
		}

		// Score: threat direction (weight 0.35).
		var threatScore float64
		if hasThreat {
			cdx := float64(x - cx)
			cdy := float64(y - cy)
			cd := math.Sqrt(cdx*cdx + cdy*cdy)
			if cd > 0 {
				dot := (cdx/cd)*threatX + (cdy/cd)*threatY
				threatScore = (dot + 1) / 2 // normalize [-1,1] to [0,1]
			}
		} else {
			threatScore = 0.5 // neutral when no intel
		}

		// Score: high-value protection (weight 0.15).
		var protectionScore float64
		if len(hvBuildings) > 0 {
			minDist := math.MaxFloat64
			for _, hv := range hvBuildings {
				dx := float64(x - hv.X)
				dy := float64(y - hv.Y)
				d := math.Sqrt(dx*dx + dy*dy)
				if d < minDist {
					minDist = d
				}
			}
			protectionScore = 1 - minDist/(2*radius)
			if protectionScore < 0 {
				protectionScore = 0
			}
		}

		// Measured to the nearest refinery specifically, so the boost pulls
		// defenses to the raided economy and not the tech/war-factory cluster.
		var refineryScore float64
		if harvesterEmergency && len(refineries) > 0 {
			minDist := math.MaxFloat64
			for _, r := range refineries {
				dx := float64(x - r.X)
				dy := float64(y - r.Y)
				d := math.Sqrt(dx*dx + dy*dy)
				if d < minDist {
					minDist = d
				}
			}
			refineryScore = 1 - minDist/radius
			if refineryScore < 0 {
				refineryScore = 0
			}
		}

		// Score: spread from existing defenses (weight 0.25).
		var spreadScore float64
		if len(defenses) > 0 {
			minDist := math.MaxFloat64
			for _, def := range defenses {
				dx := float64(x - def.X)
				dy := float64(y - def.Y)
				dist := math.Sqrt(dx*dx + dy*dy)
				if dist < minDist {
					minDist = dist
				}
			}
			spreadScore = minDist / radius
			if spreadScore > 1 {
				spreadScore = 1
			}
		} else {
			spreadScore = 1.0
		}

		// Score: perimeter bonus (weight 0.15).
		distFromCenter := math.Sqrt(float64((x-cx)*(x-cx) + (y-cy)*(y-cy)))
		perimeterScore := distFromCenter / radius
		if perimeterScore > 1 {
			perimeterScore = 1
		}

		// Score: chokepoint proximity (weight 0.20), weighted by the choke's own
		// score — a bridge on the enemy path beats an off-route land pinch.
		var chokeScore float64
		if len(nearbyChokes) > 0 {
			best := 0.0
			for _, c := range nearbyChokes {
				dx := float64(x - c.x)
				dy := float64(y - c.y)
				d := math.Sqrt(dx*dx + dy*dy)
				prox := 1 - d/(2*radius)
				if prox < 0 {
					prox = 0
				}
				s := prox * c.score
				if s > best {
					best = s
				}
			}
			chokeScore = best
		}

		// Under a raid, covering the refinery outweighs pushing the perimeter
		// toward the enemy. Spread survives so we don't wall one refinery.
		var score float64
		if harvesterEmergency {
			score = 0.50*refineryScore + 0.15*spreadScore + 0.15*threatScore + 0.10*chokeScore + 0.10*perimeterScore
		} else {
			score = 0.30*threatScore + 0.20*chokeScore + 0.15*protectionScore + 0.20*spreadScore + 0.15*perimeterScore
		}
		candidates = append(candidates, candidate{x, y, score, seed})
	}

	if len(candidates) == 0 {
		return cx, cy
	}

	slices.SortFunc(candidates, func(a, b candidate) int {
		if a.score > b.score {
			return -1
		}
		if a.score < b.score {
			return 1
		}
		return 0
	})

	top := min(3, len(candidates))
	pick := candidates[rand.Intn(top)]
	if pick.seed {
		slog.Info("defense seeded at chokepoint", "x", pick.x, "y", pick.y, "score", pick.score)
	}
	return pick.x, pick.y
}

// defenseRoles is unordered — selection is by lowest count.
var defenseRoles = []string{"pillbox", "camo_pillbox", "turret", "flame_tower", "tesla_coil"}

// repairToggleEntry gives per-building idempotency. OpenRA's repair order is a
// toggle — resending it cancels the in-flight repair — so a rule that fires
// every eval would flip repairs on and off and never complete one.
type repairToggleEntry struct {
	Tick int
}

// repairResendTicks is how long we trust an in-flight toggle before assuming
// it was lost or stopped and re-issuing.
const repairResendTicks = 1500

// isCriticalRepairType gates repair spend under pressure to production and
// economy buildings. Repairing everything starves unit production of cash.
func isCriticalRepairType(t string) bool {
	switch baseTypeName(t) {
	case ConstructionYard, WarFactory, AlliedBarracks, SovietBarracks, Refinery, PowerPlant, AdvancedPower:
		return true
	}
	return false
}

// Repair floors, by what is being repaired.
//
// One floor of 500 read against the whole base meant repair never ran. It has
// acted zero times in every game on record — 57/0, 43/0, 31/0, 14/0 across four
// consecutive games, each of them with a base visibly taking damage. The floor
// bites hardest exactly when repair matters, because a base under attack is a
// base spending its cash on defences.
//
// The reasoning behind a floor is sound: repairs charge per tick per building
// and will eat income the moment ore converts. But it was being applied to a
// war factory at a fifth health the same as to a radar dome with a scratch. A
// construction yard is worth more than the reserve it is being protected from.
const repairCashFloor = 500

const criticalRepairFloor = 150

// repairMaxConcurrent bounds cash drain: OpenRA charges per tick per active
// repair, so N damaged buildings drain N times income.
const repairMaxConcurrent = 2

// ActionRepairDamagedBuildings honors the doctrine's repair_budget_ratio knob,
// read from the active doctrine policy: the fraction of cash repair is
// allowed to touch, the rest reserved for production.
func ActionRepairDamagedBuildings(env RuleEnv, conn CommandSender) error {
	// Below even the critical floor, production and rebuild need what's left.
	if env.Cash() < criticalRepairFloor {
		return nil
	}
	repairAnything := env.Cash() >= repairCashFloor

	state := memoryMap[int, repairToggleEntry](env.Memory, "repairToggleSent")
	underPressure := env.IsRushed() || env.IsHarvesterHarassed()
	budgetRatio := env.Policy.RepairBudgetRatio
	// Zero disables the budget entirely (unlimited repair).
	reserveOK := true
	if budgetRatio > 0 && budgetRatio < 1.0 {
		// The bill is what we are about to start, not everything that is
		// damaged. Scaling it by the damaged count inverted the rule: a base
		// under heavy attack has the most damage and so demanded the most cash
		// before fixing any of it — eight damaged buildings at a ratio of 0.2
		// asked for 4000 credits against a cash median in the low hundreds.
		// Repairs are bounded by repairMaxConcurrent regardless.
		perBuildingRepairAllowance := 100 // rough estimate per repair started
		neededHeadroom := int(float64(repairMaxConcurrent*perBuildingRepairAllowance) / budgetRatio)
		if env.Cash() < neededHeadroom {
			reserveOK = false
		}
	}

	// Entries older than repairResendTicks count as stale, not active.
	activeRepairs := 0
	for _, entry := range state {
		if env.State.Tick-entry.Tick < repairResendTicks {
			activeRepairs++
		}
	}

	for _, b := range env.DamagedBuildings() {
		// Under pressure, radar/tech/helipad damage is acceptable; lost
		// production is not. The same test decides what is worth repairing on
		// thin cash: between the two floors, only the buildings that lose the
		// game when they die.
		if (underPressure || !repairAnything) && !isCriticalRepairType(b.Type) {
			continue
		}
		// Budget gates new repairs only; in-flight ones still complete. It does
		// not gate the buildings that lose the game when they die: a doctrine
		// choosing to spend a fifth of its cash on repairs is choosing between
		// a radar dome and a tank, not between a construction yard and one.
		prev, sent := state[b.ID]
		if !reserveOK && !sent && !isCriticalRepairType(b.Type) {
			continue
		}
		if sent && env.State.Tick-prev.Tick < repairResendTicks {
			continue
		}
		// Rate-limits new starts only; tracked repairs finish on their own.
		if !sent && activeRepairs >= repairMaxConcurrent {
			continue
		}
		state[b.ID] = repairToggleEntry{Tick: env.State.Tick}
		if !sent {
			activeRepairs++
		}
		slog.Debug("repairing building", "id", b.ID, "type", b.Type)
		if err := conn.Send(ipc.TypeRepairBuilding, ipc.RepairBuildingCommand{
			ActorID: uint32(b.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

type egressEntry struct{ Tick int }

// ActionUnblockWarFactoryEgress scatters a friendly unit camped on the war
// factory exit. A vehicle stuck behind one hangs the Vehicle queue at 100% and
// every produce rule downstream of it stalls for the rest of the game.
//
// The blocker is pushed along the factory-to-centroid axis so it moves into the
// base interior rather than back across the tile it was blocking.
func ActionUnblockWarFactoryEgress(env RuleEnv, conn CommandSender) error {
	wf := env.WarFactory()
	if wf == nil {
		return nil
	}
	var blocker *model.Unit
	bestDist := math.MaxFloat64
	for i := range env.State.Units {
		u := &env.State.Units[i]
		if matchesType(u.Type, Harvester) || matchesType(u.Type, MCV) {
			continue
		}
		dx := float64(u.X - wf.X)
		dy := float64(u.Y - wf.Y)
		d := math.Sqrt(dx*dx + dy*dy)
		if d <= 3.0 && d < bestDist {
			bestDist = d
			blocker = u
		}
	}
	if blocker == nil {
		return nil
	}
	state := memoryMap[int, egressEntry](env.Memory, "egressNudged")
	if prev, ok := state[blocker.ID]; ok && env.State.Tick-prev.Tick < unblockEgressResend {
		return nil
	}
	cx, cy := env.BuildingCentroid()
	dx := cx - wf.X
	dy := cy - wf.Y
	dist := math.Sqrt(float64(dx*dx + dy*dy))
	const pushRange = 6
	var tx, ty int
	if dist < 0.5 {
		tx = wf.X + pushRange
		ty = wf.Y + pushRange
	} else {
		tx = wf.X + int(math.Round(float64(dx)/dist*float64(pushRange)))
		ty = wf.Y + int(math.Round(float64(dy)/dist*float64(pushRange)))
	}
	state[blocker.ID] = egressEntry{Tick: env.State.Tick}
	slog.Info("unblocking war factory egress", "blocker", blocker.ID, "type", blocker.Type,
		"from", []int{blocker.X, blocker.Y}, "to", []int{tx, ty})
	return conn.Send(ipc.TypeMove, ipc.MoveCommand{
		ActorID: uint32(blocker.ID),
		X:       tx,
		Y:       ty,
	})
}
