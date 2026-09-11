package rules

import (
	"log/slog"
	"math"
	"math/rand"
	"reflect"
	"slices"
	"strings"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

func ActionProduceMCV(env RuleEnv, conn *ipc.Connection) error {
	slog.Debug("producing MCV — construction yard lost")
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  MCV,
		Count: 1,
	})
}

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

func ActionDeployMCV(env RuleEnv, conn *ipc.Connection) error {
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

func ActionProducePowerPlant(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("power_plant")
	if item == "" {
		return nil
	}
	slog.Debug("producing power plant", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceRefinery(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("refinery")
	if item == "" {
		return nil
	}
	slog.Debug("producing refinery", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceBarracks(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("barracks")
	if item == "" {
		return nil
	}
	slog.Debug("producing barracks", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceWarFactory(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("war_factory")
	if item == "" {
		return nil
	}
	slog.Debug("producing war factory", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceRadar(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("radar")
	if item == "" {
		return nil
	}
	slog.Debug("producing radar dome", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceAirfield(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("airfield")
	if item == "" {
		return nil
	}
	slog.Debug("producing airfield", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceServiceDepot(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("service_depot")
	if item == "" {
		return nil
	}
	slog.Debug("producing service depot", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceNavalYard(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("naval_yard")
	if item == "" {
		return nil
	}
	slog.Debug("producing naval yard", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

// ActionCancelStuckAircraft works around aircraft production that completes but
// can't spawn for want of a free pad, blocking the queue.
func ActionCancelStuckAircraft(env RuleEnv, conn *ipc.Connection) error {
	for _, pq := range env.State.ProductionQueues {
		if strings.EqualFold(pq.Type, QueueAircraft) && pq.CurrentItem != "" && pq.CurrentProgress >= 100 {
			slog.Info("cancelling stuck aircraft production", "item", pq.CurrentItem)
			return conn.Send(ipc.TypeCancelProduction, ipc.CancelProductionCommand{
				Queue: QueueAircraft,
				Item:  pq.CurrentItem,
				Count: 1,
			})
		}
	}
	return nil
}

func ActionPlaceBuilding(env RuleEnv, conn *ipc.Connection) error {
	for _, pq := range env.State.ProductionQueues {
		if strings.EqualFold(pq.Type, QueueBuilding) && pq.CurrentItem != "" && pq.CurrentProgress >= 100 {
			slog.Debug("placing building", "item", pq.CurrentItem)
			return conn.Send(ipc.TypePlaceBuilding, ipc.PlaceBuildingCommand{
				Queue: QueueBuilding,
				Item:  pq.CurrentItem,
			})
		}
	}
	return nil
}

func ActionProduceInfantry(env RuleEnv, conn *ipc.Connection) error {
	slog.Debug("producing infantry")
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueInfantry,
		Item:  RifleInfantry,
		Count: 1,
	})
}

func ActionProduceVehicle(env RuleEnv, conn *ipc.Connection) error {
	item := env.BestBuildableVehicle()
	if item == "" {
		return nil
	}
	slog.Debug("producing vehicle", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceSpecialistInfantry(env RuleEnv, conn *ipc.Connection) error {
	item := env.BestBuildableSpecialist()
	if item == "" {
		return nil
	}
	slog.Debug("producing specialist infantry", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueInfantry,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceAircraft(env RuleEnv, conn *ipc.Connection) error {
	item := env.BestBuildableAircraft()
	if item == "" {
		return nil
	}
	slog.Debug("producing aircraft", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueAircraft,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceShip(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("submarine")
	if item == "" {
		item = env.BuildableType("destroyer")
	}
	if item == "" {
		return nil
	}
	slog.Debug("producing ship", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueShip,
		Item:  item,
		Count: 1,
	})
}

func ActionPlaceDefense(env RuleEnv, conn *ipc.Connection) error {
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
	buildings := env.State.Buildings
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

func ActionProduceDefense(env RuleEnv, conn *ipc.Connection) error {
	// Fewest-of-type, so the mix diversifies instead of stacking whichever
	// type happens to be listed first.
	bestRole := ""
	bestItem := ""
	bestCount := math.MaxInt
	for _, role := range defenseRoles {
		item := env.BuildableType(role)
		if item == "" {
			continue
		}
		count := env.RoleCount(role)
		if count < bestCount {
			bestCount = count
			bestRole = role
			bestItem = item
		}
	}
	if bestItem == "" {
		return nil
	}
	slog.Debug("producing defense", "role", bestRole, "item", bestItem, "existing", bestCount)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueDefense,
		Item:  bestItem,
		Count: 1,
	})
}

func ActionProduceAADefense(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("aa_defense")
	if item == "" {
		return nil
	}
	slog.Debug("producing AA defense", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueDefense,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceGapGenerator(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("gap_generator")
	if item == "" {
		return nil
	}
	slog.Debug("producing gap generator", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueDefense,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceTechCenter(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("tech_center")
	if item == "" {
		return nil
	}
	slog.Debug("producing tech center", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceFlameTower(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("flame_tower")
	if item == "" {
		return nil
	}
	slog.Debug("producing flame tower", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueDefense,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceTeslaCoil(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("tesla_coil")
	if item == "" {
		return nil
	}
	slog.Debug("producing tesla coil", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueDefense,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceHeavyVehicle(env RuleEnv, conn *ipc.Connection) error {
	for _, role := range []string{"heavy_tank", "medium_tank"} {
		item := env.BuildableType(role)
		if item != "" {
			slog.Debug("producing heavy vehicle", "item", item)
			return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
				Queue: QueueVehicle,
				Item:  item,
				Count: 1,
			})
		}
	}
	return nil
}

func ActionProduceScoutVehicle(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("ranger")
	if item == "" {
		// Soviets don't have rangers — use a light tank as scout.
		item = env.BuildableType("light_tank")
	}
	if item == "" {
		return nil
	}
	slog.Debug("producing scout vehicle", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceSiegeVehicle(env RuleEnv, conn *ipc.Connection) error {
	for _, role := range []string{"artillery", "v2_launcher"} {
		if item := env.BuildableType(role); item != "" {
			slog.Debug("producing siege vehicle", "item", item)
			return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
				Queue: QueueVehicle,
				Item:  item,
				Count: 1,
			})
		}
	}
	return nil
}

func ActionProduceBasicAircraft(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("basic_aircraft")
	if item == "" {
		return nil
	}
	slog.Debug("producing basic aircraft", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueAircraft,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceRocketSoldier(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("rocket_soldier")
	if item == "" {
		return nil
	}
	slog.Debug("producing rocket soldier", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueInfantry,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceAdvancedAircraft(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("advanced_aircraft")
	if item == "" {
		return nil
	}
	slog.Debug("producing advanced aircraft", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueAircraft,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceAdvancedPower(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("advanced_power")
	if item == "" {
		return nil
	}
	slog.Debug("producing advanced power", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceOreSilo(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("ore_silo")
	if item == "" {
		return nil
	}
	slog.Debug("producing ore silo", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceAdvancedShip(env RuleEnv, conn *ipc.Connection) error {
	for _, role := range []string{"cruiser", "missile_sub", "destroyer"} {
		item := env.BuildableType(role)
		if item != "" {
			slog.Debug("producing advanced ship", "item", item)
			return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
				Queue: QueueShip,
				Item:  item,
				Count: 1,
			})
		}
	}
	return nil
}

func ActionDefendBase(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	// Unassigned only. Squad members are reserved for the push; poaching them
	// for every lone raider means the attack squad never deploys. Base defense
	// belongs to the ground-defense squad.
	idle := env.UnassignedIdleGround()
	if len(idle) == 0 {
		return nil
	}
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("defending base", "count", len(ids), "target", enemy.ID)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        enemy.X,
		Y:        enemy.Y,
	})
}

// ActionDefendCriticalBuilding pulls every nearby ground unit, squad members
// included. This deliberately overrides the poach-prevention elsewhere: with
// the CY or war factory actively burning, reserving a squad for a future push
// costs more than it buys.
func ActionDefendCriticalBuilding(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.nearestEnemyAttackingCritical()
	if enemy == nil {
		return nil
	}
	nearby := env.NearBaseGroundUnits()
	if len(nearby) == 0 {
		return nil
	}
	ids := make([]uint32, len(nearby))
	for i, u := range nearby {
		ids[i] = uint32(u.ID)
	}
	slog.Info("defending critical building — engaging attacker", "count", len(ids), "target", enemy.ID, "targetType", enemy.Type)
	return sendAttackMove(env, conn, ids, enemy.X, enemy.Y)
}

// ActionEmergencyDefendBase covers the case where nothing is idle but nearby
// units are sitting on stale orders while the base is attacked.
func ActionEmergencyDefendBase(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	nearby := env.NearBaseGroundUnits()
	if len(nearby) == 0 {
		return nil
	}
	ids := make([]uint32, len(nearby))
	for i, u := range nearby {
		ids[i] = uint32(u.ID)
	}
	slog.Info("emergency base defense — recalling nearby units", "count", len(ids), "target", enemy.ID)
	return sendAttackMove(env, conn, ids, enemy.X, enemy.Y)
}

func ActionNavalDefendBase(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	idle := env.IdleNavalUnits()
	if len(idle) == 0 {
		return nil
	}
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("naval defending base", "count", len(ids), "target", enemy.ID)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        enemy.X,
		Y:        enemy.Y,
	})
}

func ActionAirDefendBase(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	for _, u := range env.IdleCombatAircraft() {
		slog.Debug("air defend", "aircraft", u.ID, "target", enemy.ID)
		if err := conn.Send(ipc.TypeAttack, ipc.AttackCommand{
			ActorID:  uint32(u.ID),
			TargetID: uint32(enemy.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

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

// repairCashFloor stops all repair spend below this cash level. Without it,
// in-flight repairs consume income the instant ore converts and cash sits
// pinned at zero for the whole mid-game.
const repairCashFloor = 500

// repairMaxConcurrent bounds cash drain: OpenRA charges per tick per active
// repair, so N damaged buildings drain N times income.
const repairMaxConcurrent = 2

// ActionRepairDamagedBuildings honors the doctrine's repair_budget_ratio knob,
// read from env memory as "repairBudgetRatio": the fraction of cash repair is
// allowed to touch, the rest reserved for production.
func ActionRepairDamagedBuildings(env RuleEnv, conn *ipc.Connection) error {
	// Below the floor, production and rebuild need what's left.
	if env.State.Player.Cash < repairCashFloor {
		return nil
	}

	state := memoryMap[int, repairToggleEntry](env.Memory, "repairToggleSent")
	underPressure := env.IsRushed() || env.IsHarvesterHarassed()
	budgetRatio, _ := env.Memory["repairBudgetRatio"].(float64)
	// Zero disables the budget entirely (unlimited repair).
	reserveOK := true
	if budgetRatio > 0 && budgetRatio < 1.0 {
		// Approximation: gate new starts on current cash clearing a reserve
		// that scales with damaged-building count as a proxy for total cost.
		damagedCount := len(env.DamagedBuildings())
		perBuildingRepairAllowance := 100 // rough estimate per damaged building
		neededHeadroom := int(float64(damagedCount*perBuildingRepairAllowance) / budgetRatio)
		if env.State.Player.Cash < neededHeadroom {
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
		// production is not.
		if underPressure && !isCriticalRepairType(b.Type) {
			continue
		}
		// Budget gates new repairs only; in-flight ones still complete.
		prev, sent := state[b.ID]
		if !reserveOK && !sent {
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

// ActionScoutWithIdleUnits farms up to 2 idle ground units for perimeter recon
// when no enemy is visible. AttackMove so scouts engage what they spot en
// route. Patrol assignments persist per scout: reissuing cancels the in-flight
// path, and a stateless version left scouts bouncing between corners.
func ActionScoutWithIdleUnits(env RuleEnv, conn *ipc.Connection) error {
	waypoints := generateWaypoints(env.State.MapWidth, env.State.MapHeight, env.Terrain)
	if len(waypoints) == 0 {
		return nil
	}
	idle := env.IdleGroundUnits()
	n := min(2, len(idle))
	if n == 0 {
		return nil
	}
	state := memoryMap[int, scoutMoveEntry](env.Memory, "idleScoutMoveSent")

	for i := 0; i < n; i++ {
		u := idle[i]
		prev, assigned := state[u.ID]

		forceAdvance := false
		if assigned && actorStalledAt(env.Memory, "idleScoutProgress", u, env.State.Tick, scoutStallTicks) {
			forceAdvance = true
			delete(memoryMap[int, apcProgressEntry](env.Memory, "idleScoutProgress"), u.ID)
		}
		idx := nextPatrolIdx(u.X, u.Y, prev.X, prev.Y, prev.Idx, len(waypoints), scoutArriveRadius, assigned, forceAdvance)
		if !assigned {
			idx = takePatrolPoolIdx(env.Memory, "idleScoutPatrolIdx", len(waypoints))
		}
		wp := waypoints[idx%len(waypoints)]

		if assigned && prev.X == wp[0] && prev.Y == wp[1] && env.State.Tick-prev.Tick < scoutMoveResend {
			continue
		}
		state[u.ID] = scoutMoveEntry{Tick: env.State.Tick, X: wp[0], Y: wp[1], Idx: idx}
		slog.Debug("scouting with idle unit", "id", u.ID, "type", u.Type, "waypoint", wp, "wpIdx", idx)
		if err := conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: []uint32{uint32(u.ID)},
			X:        wp[0],
			Y:        wp[1],
		}); err != nil {
			return err
		}
	}
	return nil
}

// generateWaypoints lays a 9-point search pattern, inset from the edges to
// avoid map-boundary pathing trouble and filtered against terrain so ground
// scouts only get reachable targets.
func generateWaypoints(mapW, mapH int, terrain *model.TerrainGrid) [][2]int {
	if mapW == 0 || mapH == 0 {
		return nil
	}
	marginX := max(3, mapW/25)
	marginY := max(3, mapH/25)
	minX, maxX := marginX, mapW-marginX
	minY, maxY := marginY, mapH-marginY
	midX := mapW / 2
	midY := mapH / 2

	candidates := [][2]int{
		{midX, midY}, // center
		{minX, minY}, // top-left
		{maxX, minY}, // top-right
		{maxX, maxY}, // bottom-right
		{minX, maxY}, // bottom-left
		{midX, minY}, // top-mid
		{maxX, midY}, // right-mid
		{midX, maxY}, // bottom-mid
		{minX, midY}, // left-mid
	}

	if terrain == nil {
		return candidates
	}

	var filtered [][2]int
	for _, wp := range candidates {
		t := terrain.AtMapPos(wp[0], wp[1])
		if t == model.Land || t == model.Bridge {
			filtered = append(filtered, wp)
		}
	}
	if len(filtered) == 0 {
		return candidates // fallback: don't leave scouts with zero waypoints
	}
	return filtered
}

// ActionScoutPatrol rotates each idle scout through perimeter waypoints, one
// at a time until it arrives. Moves are throttled: re-issuing a destination
// cancels the in-flight path, and scouts then never traverse the map.
func ActionScoutPatrol(env RuleEnv, conn *ipc.Connection) error {
	waypoints := generateWaypoints(env.State.MapWidth, env.State.MapHeight, env.Terrain)
	if len(waypoints) == 0 {
		return nil
	}

	scouts := env.IdleScouts()
	state := getScoutMoveState(env.Memory)

	// A rush needs the enemy base found fast; once we know where it is, the
	// perimeter rotation is spent on corners that no longer matter. But the
	// waypoint is the base CENTRE, so this has to end when the search does:
	// HasEnemyIntel means we have actually seen the buildings, and driving at
	// them again only feeds the defenses. Game 102 held scout-reach above 0.5
	// in all thirty windows and sent its one light tank in until it died.
	scoutReach, _ := env.Memory["scoutReachPriority"].(float64)
	targetKnownBase := scoutReach > 0.5 && !env.HasEnemyIntel() && env.NearestEnemyBase() != nil

	for _, s := range scouts {
		prev, assigned := state[s.ID]
		// A scout that hasn't moved is blocked (chokepoint, terrain lock), not
		// slow — advance rather than retry the same target for the whole game.
		forceAdvance := false
		if assigned && scoutStalled(env.Memory, s, env.State.Tick) {
			forceAdvance = true
			delete(memoryMap[int, apcProgressEntry](env.Memory, "scoutProgress"), s.ID)
		}
		idx := nextPatrolIdx(s.X, s.Y, prev.X, prev.Y, prev.Idx, len(waypoints), scoutArriveRadius, assigned, forceAdvance)
		if !assigned {
			idx = takePatrolPoolIdx(env.Memory, "scoutPatrolIdx", len(waypoints))
		}
		wp := waypoints[idx%len(waypoints)]
		// Throttle and stall machinery still apply — the destination is compared
		// against prev.X/prev.Y either way.
		if targetKnownBase {
			base := env.NearestEnemyBase()
			wp = [2]int{base.X, base.Y}
		}

		// The path is still valid; a fresh Move would cancel it.
		if assigned && prev.X == wp[0] && prev.Y == wp[1] && env.State.Tick-prev.Tick < scoutMoveResend {
			continue
		}
		state[s.ID] = scoutMoveEntry{Tick: env.State.Tick, X: wp[0], Y: wp[1], Idx: idx}
		slog.Debug("scout patrolling", "id", s.ID, "type", s.Type, "waypoint", wp, "wpIdx", idx)
		if err := conn.Send(ipc.TypeMove, ipc.MoveCommand{
			ActorID: uint32(s.ID),
			X:       wp[0],
			Y:       wp[1],
		}); err != nil {
			return err
		}
	}
	return nil
}

// nextPatrolIdx advances an actor's round-robin waypoint index on arrival, or
// when the caller forces it (stall, TTL). Shared by scouts and APCs in scout
// mode so both rotate systematically and leave no blind spots.
//
// The return is meaningless when assigned is false — first assignments come
// from takePatrolPoolIdx instead.
func nextPatrolIdx(actorX, actorY, prevX, prevY, prevIdx, numWaypoints int, arriveRadius float64, assigned, forceAdvance bool) int {
	if !assigned {
		return prevIdx
	}
	if forceAdvance {
		return (prevIdx + 1) % numWaypoints
	}
	dx := float64(actorX - prevX)
	dy := float64(actorY - prevY)
	if math.Sqrt(dx*dx+dy*dy) < arriveRadius {
		return (prevIdx + 1) % numWaypoints
	}
	return prevIdx
}

// takePatrolPoolIdx fans out actors assigned on the same tick so they don't
// all start on waypoint 0.
func takePatrolPoolIdx(memory map[string]any, key string, numWaypoints int) int {
	pool, _ := memory[key].(int)
	memory[key] = (pool + 1) % numWaypoints
	return pool % numWaypoints
}

type scoutMoveEntry struct {
	Tick int
	X    int
	Y    int
	Idx  int
}

// scoutMoveResend is shorter than the APC window — scouts must keep moving, and
// a scout that arrives rotates to a different destination anyway, which bypasses
// the throttle.
const scoutMoveResend = 40
const scoutArriveRadius = 6.0

// lightTankScoutMinVehicles is the armour that must remain before a light tank
// is worth spending on patrol.
const lightTankScoutMinVehicles = 3

func getScoutMoveState(memory map[string]any) map[int]scoutMoveEntry {
	return memoryMap[int, scoutMoveEntry](memory, "scoutMoveSent")
}

func ActionProduceEngineer(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("engineer")
	if item == "" {
		return nil
	}
	slog.Debug("producing engineer", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueInfantry,
		Item:  item,
		Count: 1,
	})
}

// captureOrderEntry throttles repeat Capture orders. Same reason as
// sendAPCMove: a fresh Capture cancels the in-flight walk-and-capture, so an
// engineer re-ordered every tick never arrives.
type captureOrderEntry struct {
	Tick     int
	TargetID int
}

const captureOrderResend = 60

func getCaptureOrderState(memory map[string]any) map[int]captureOrderEntry {
	return memoryMap[int, captureOrderEntry](memory, "captureSent")
}

func ActionCaptureBuilding(env RuleEnv, conn *ipc.Connection) error {
	target := env.NearestCapturable()
	if target == nil {
		return nil
	}
	engineers := env.IdleEngineers()
	if len(engineers) == 0 {
		return nil
	}
	// Closest first, so a just-unloaded engineer captures rather than being
	// re-loaded into another APC.
	eng, _ := nearestTo(engineers, target.X, target.Y)

	state := getCaptureOrderState(env.Memory)
	if prev, ok := state[eng.ID]; ok && prev.TargetID == target.ID && env.State.Tick-prev.Tick < captureOrderResend {
		return nil
	}
	state[eng.ID] = captureOrderEntry{Tick: env.State.Tick, TargetID: target.ID}

	slog.Debug("capturing building", "engineer", eng.ID, "target", target.ID, "type", target.Type)
	return conn.Send(ipc.TypeCapture, ipc.CaptureCommand{
		ActorID:  uint32(eng.ID),
		TargetID: uint32(target.ID),
	})
}

func ActionProduceHarvester(env RuleEnv, conn *ipc.Connection) error {
	slog.Debug("producing harvester")
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  Harvester,
		Count: 1,
	})
}

// harvestResend throttles repeat harvest orders. A harvester idle for a single
// tick (waiting to dock, between trips) would otherwise get a fresh order that
// cancels its pathing and strands it beside the refinery.
const harvestResend = 120

type harvestEntry struct {
	Tick int
	X    int
	Y    int
}

func ActionSendIdleHarvesters(env RuleEnv, conn *ipc.Connection) error {
	// Round-robin across refineries so idle harvesters spread out instead of
	// converging on the first one's coordinates.
	var refineries []model.Building
	for i := range env.State.Buildings {
		if matchesType(env.State.Buildings[i].Type, Refinery) {
			refineries = append(refineries, env.State.Buildings[i])
		}
	}
	// Skip harvesters the flee action is moving. Both rules fire on the same
	// tick in different categories, so exclusivity can't arbitrate, and each
	// sends the harvester to the nearest refinery — the result is a shuttle
	// between fleeing and returning to danger.
	//
	// Liveness is judged by entry age, not by the map being pruned: the pruning
	// rule only runs while something is in danger, so trusting the map's
	// contents froze every harvester that had ever fled.
	fleeing := getHarvesterFleeState(env.Memory)

	// Only rescue harvesters that have been idle a while.
	//
	// This sends a Harvest order at a REFINERY's position, because ore is not
	// in the game state at all — the sidecar is observation-only and ore must
	// be scouted. So the order means "mine near this refinery", and when that
	// patch is exhausted the harvester finds nothing, goes idle, and is ordered
	// there again. Game 96: return-idle-harvesters matched 746 times, and the
	// harvesters sat in a heap beside the refineries while ore lay elsewhere.
	//
	// The engine knows where ore is and we do not, so it gets first refusal.
	// An order issued the moment a harvester goes idle interrupts that search;
	// one issued after it has been idle for a while is a genuine rescue.
	idleSince := memoryMap[int, int](env.Memory, "harvesterIdleSince")
	stillIdle := map[int]bool{}
	for _, u := range env.IdleHarvesters() {
		stillIdle[u.ID] = true
		if _, seen := idleSince[u.ID]; !seen {
			idleSince[u.ID] = env.State.Tick
		}
	}
	for id := range idleSince {
		if !stillIdle[id] {
			delete(idleSince, id)
		}
	}

	state := memoryMap[int, harvestEntry](env.Memory, "harvestSent")
	for i, u := range env.IdleHarvesters() {
		// The grace period is for a harvester whose idleness we cannot explain,
		// so the engine gets a chance to sort it out. One that has just fled is
		// idle for a reason we know: it ran to a refinery and stopped. Making it
		// wait is pure lost mining — game 97 fled 116 times, and at 250 ticks
		// each that is a large share of a 27,000-tick game's harvesting.
		_, justFled := fleeing[u.ID]
		if since, ok := idleSince[u.ID]; ok && !justFled && env.State.Tick-since < harvesterIdleGrace {
			continue
		}
		if prev, ok := fleeing[u.ID]; ok && env.State.Tick-prev.Tick < harvesterFleeResend {
			continue
		}
		var tx, ty int
		switch {
		case len(refineries) > 0:
			r := refineries[i%len(refineries)]
			tx, ty = r.X, r.Y
		case len(env.State.Buildings) > 0:
			tx, ty = env.State.Buildings[0].X, env.State.Buildings[0].Y
		}
		if prev, ok := state[u.ID]; ok && prev.X == tx && prev.Y == ty && env.State.Tick-prev.Tick < harvestResend {
			continue
		}
		state[u.ID] = harvestEntry{Tick: env.State.Tick, X: tx, Y: ty}
		slog.Debug("sending idle harvester", "id", u.ID, "x", tx, "y", ty)
		if err := conn.Send(ipc.TypeHarvest, ipc.HarvestCommand{
			ActorID: uint32(u.ID),
			X:       tx,
			Y:       ty,
		}); err != nil {
			return err
		}
	}
	return nil
}

func ActionAttackMoveIdleGroundUnits(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	idle := env.IdleGroundUnits()
	if len(idle) == 0 {
		return nil
	}
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("attack-moving idle ground units", "count", len(ids), "target", enemy.ID)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        enemy.X,
		Y:        enemy.Y,
	})
}

func ActionAttackKnownBaseGround(env RuleEnv, conn *ipc.Connection) error {
	base := env.NearestEnemyBase()
	if base == nil {
		return nil
	}
	idle := env.IdleGroundUnits()
	if len(idle) == 0 {
		return nil
	}
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("ground attacking known enemy base", "count", len(ids), "owner", base.Owner, "x", base.X, "y", base.Y)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        base.X,
		Y:        base.Y,
	})
}

func ActionAirAttackEnemy(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	for _, u := range env.IdleCombatAircraft() {
		slog.Debug("air attack enemy", "aircraft", u.ID, "target", enemy.ID)
		if err := conn.Send(ipc.TypeAttack, ipc.AttackCommand{
			ActorID:  uint32(u.ID),
			TargetID: uint32(enemy.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func ActionAirAttackKnownBase(env RuleEnv, conn *ipc.Connection) error {
	base := env.NearestEnemyBase()
	if base == nil {
		return nil
	}
	aircraft := env.IdleCombatAircraft()
	if len(aircraft) == 0 {
		return nil
	}
	ids := make([]uint32, len(aircraft))
	for i, u := range aircraft {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("air attacking known enemy base", "count", len(ids), "owner", base.Owner, "x", base.X, "y", base.Y)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        base.X,
		Y:        base.Y,
	})
}

// --- Superweapon building production ---
// Defense queue despite being buildings — OpenRA categorizes them that way.

func ActionProduceMissileSilo(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("missile_silo")
	if item == "" {
		return nil
	}
	slog.Debug("producing missile silo", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueDefense,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceIronCurtain(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("iron_curtain")
	if item == "" {
		return nil
	}
	slog.Debug("producing iron curtain", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueDefense,
		Item:  item,
		Count: 1,
	})
}

// --- Superweapon fire actions ---

func ActionFireNuke(env RuleEnv, conn *ipc.Connection) error {
	x, y := 0, 0
	if base := env.NearestEnemyBase(); base != nil {
		x, y = base.X, base.Y
	} else if enemy := env.NearestEnemy(); enemy != nil {
		x, y = enemy.X, enemy.Y
	} else {
		x, y = env.MapWidth()/2, env.MapHeight()/2
	}
	recordSuperweaponFire(env, "nuke")
	slog.Info("firing nuke", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "NukePowerInfoOrder",
		X:        x,
		Y:        y,
	})
}

func ActionFireIronCurtain(env RuleEnv, conn *ipc.Connection) error {
	x, y := env.GroundUnitCentroid()
	recordSuperweaponFire(env, "iron_curtain")
	slog.Info("firing iron curtain on own units", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "GrantExternalConditionPowerInfoOrder",
		X:        x,
		Y:        y,
	})
}

func ActionFireSpyPlane(env RuleEnv, conn *ipc.Connection) error {
	x, y := 0, 0
	if base := env.NearestEnemyBase(); base != nil {
		x, y = base.X, base.Y
	} else {
		x, y = env.MapWidth()/2, env.MapHeight()/2
	}
	recordSuperweaponFire(env, "spy_plane")
	slog.Info("firing spy plane", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "SovietSpyPlane",
		X:        x,
		Y:        y,
	})
}

func ActionFireParatroopers(env RuleEnv, conn *ipc.Connection) error {
	x, y := 0, 0
	if base := env.NearestEnemyBase(); base != nil && env.IsLandAt(base.X, base.Y) {
		x, y = base.X, base.Y
	} else if enemy := env.NearestEnemy(); enemy != nil && env.IsLandAt(enemy.X, enemy.Y) {
		x, y = enemy.X, enemy.Y
	} else {
		return nil // no valid land target
	}
	recordSuperweaponFire(env, "paratroopers")
	slog.Info("firing paratroopers", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "SovietParatroopers",
		X:        x,
		Y:        y,
	})
}

func ActionFireParabombs(env RuleEnv, conn *ipc.Connection) error {
	x, y := 0, 0
	if base := env.NearestEnemyBase(); base != nil && env.IsLandAt(base.X, base.Y) {
		x, y = base.X, base.Y
	} else if enemy := env.NearestEnemy(); enemy != nil && env.IsLandAt(enemy.X, enemy.Y) {
		x, y = enemy.X, enemy.Y
	} else {
		return nil // no valid land target
	}
	recordSuperweaponFire(env, "parabombs")
	slog.Info("firing parabombs", "x", x, "y", y)
	return conn.Send(ipc.TypeSupportPower, ipc.SupportPowerCommand{
		PowerKey: "UkraineParabombs",
		X:        x,
		Y:        y,
	})
}

func ActionProduceFlakTruck(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("flak_truck")
	if item == "" {
		return nil
	}
	slog.Debug("producing flak truck", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceGunboat(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("gunboat")
	if item == "" {
		return nil
	}
	slog.Debug("producing gunboat", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueShip,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceAPC(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("apc")
	if item == "" {
		item = env.BuildableType("ranger")
	}
	if item == "" {
		return nil
	}
	slog.Debug("producing transport", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceGrenadier(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("grenadier")
	if item == "" {
		return nil
	}
	slog.Debug("producing grenadier", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueInfantry,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceAttackDog(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("attack_dog")
	if item == "" {
		return nil
	}
	slog.Debug("producing attack dog", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueInfantry,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceSpy(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("spy")
	if item == "" {
		return nil
	}
	slog.Debug("producing spy", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueInfantry,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceMADTank(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("mad_tank")
	if item == "" {
		return nil
	}
	slog.Debug("producing MAD tank", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceMinelayer(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("minelayer")
	if item == "" {
		return nil
	}
	slog.Debug("producing minelayer", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceKennel(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("kennel")
	if item == "" {
		return nil
	}
	slog.Debug("producing kennel", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

// minelayerTarget lets updateMinelayers recognize a completed minefield (idle
// near its target) and free the unit. Without it a successful minelayer stays
// flagged assigned for the rest of the game.
type minelayerTarget struct {
	X, Y       int
	AssignedAt int
}

func getMinelayerTargets(memory map[string]any) map[int]minelayerTarget {
	return memoryMap[int, minelayerTarget](memory, "minelayerTargets")
}

// minelayerDoneRadius is small because the minefield is 3x3 around the target:
// idle inside it means the mines are laid.
const minelayerDoneRadius = 4

// ActionLayMines sends idle minelayers out, preferring targets in descending
// order of how much traffic they funnel: ranked chokepoints, then a point
// partway toward known enemy intel, then a blind compass perimeter.
// Rearming is OpenRA's LayMines activity, not ours.
func ActionLayMines(env RuleEnv, conn *ipc.Connection) error {
	miners := env.IdleMinelayers()
	if len(miners) == 0 {
		return nil
	}

	centX, centY := env.BuildingCentroid()
	assigned := getMinelayerAssignments(env.Memory)
	targets := getMinelayerTargets(env.Memory)

	base := env.NearestEnemyBase()
	chokes := env.ChokepointsTowardEnemy()

	// Rotating cursor so re-tasks cycle chokes instead of re-mining the last.
	chokeIdx, _ := env.Memory["mineChokeIdx"].(int)

	for i, m := range miners {
		var tx, ty int

		switch {
		case len(chokes) > 0 && env.Terrain != nil:
			c := chokes[(chokeIdx+i)%len(chokes)]
			tx, ty = env.Terrain.ZoneCenter(c.Col, c.Row)
		case base != nil:
			fraction := 0.25 + 0.05*float64(i)
			if fraction > 0.40 {
				fraction = 0.40
			}
			tx = centX + int(float64(base.X-centX)*fraction)
			ty = centY + int(float64(base.Y-centY)*fraction)
		default:
			// No intel — perimeter at ~15% of map diagonal, one compass
			// direction per minelayer.
			mw := float64(env.State.MapWidth)
			mh := float64(env.State.MapHeight)
			perimeterDist := math.Sqrt(mw*mw+mh*mh) * 0.15
			angle := float64(i) * (2 * math.Pi / 4) // N, E, S, W
			tx = centX + int(perimeterDist*math.Cos(angle))
			ty = centY + int(perimeterDist*math.Sin(angle))
		}

		tx = max(1, min(tx, env.State.MapWidth-2))
		ty = max(1, min(ty, env.State.MapHeight-2))

		const radius = 1
		startX := max(0, tx-radius)
		startY := max(0, ty-radius)
		endX := min(env.State.MapWidth-1, tx+radius)
		endY := min(env.State.MapHeight-1, ty+radius)

		slog.Debug("laying minefield", "minelayer", m.ID, "start_x", startX, "start_y", startY, "end_x", endX, "end_y", endY)
		if err := conn.Send(ipc.TypePlaceMinefield, ipc.PlaceMinefieldCommand{
			ActorID: uint32(m.ID),
			StartX:  startX,
			StartY:  startY,
			EndX:    endX,
			EndY:    endY,
		}); err != nil {
			return err
		}

		assigned[m.ID] = true
		targets[m.ID] = minelayerTarget{X: tx, Y: ty, AssignedAt: env.State.Tick}
	}

	env.Memory["minelayerAssigned"] = assigned
	env.Memory["mineChokeIdx"] = chokeIdx + len(miners)
	return nil
}

// updateMinelayers frees assignments for dead minelayers and for ones that
// have finished laying. Called each tick from the engine.
func updateMinelayers(env RuleEnv) {
	assigned := getMinelayerAssignments(env.Memory)
	if len(assigned) == 0 {
		return
	}
	targets := getMinelayerTargets(env.Memory)

	unitByID := make(map[int]model.Unit, len(env.State.Units))
	for _, u := range env.State.Units {
		unitByID[u.ID] = u
	}

	for id := range assigned {
		u, alive := unitByID[id]
		if !alive {
			delete(assigned, id)
			delete(targets, id)
			continue
		}
		t, hasTarget := targets[id]
		if !hasTarget {
			// No coords (reloaded state) — leave it; the next retask records one.
			continue
		}
		// Idle near its target means the mines are down; clear both maps so the
		// unit becomes retaskable.
		dx := float64(u.X - t.X)
		dy := float64(u.Y - t.Y)
		if u.Idle && math.Sqrt(dx*dx+dy*dy) <= minelayerDoneRadius {
			delete(assigned, id)
			delete(targets, id)
		}
	}
	env.Memory["minelayerAssigned"] = assigned
}

func ActionLoadEngineerIntoAPC(env RuleEnv, conn *ipc.Connection) error {
	engineers := env.IdleEngineers()
	if len(engineers) == 0 {
		return nil
	}
	apcs := env.IdleEmptyAPCs()
	if len(apcs) == 0 {
		return nil
	}
	GetAPCCargoIntent(env.Memory)[apcs[0].ID] = apcIntentEngineer
	slog.Debug("loading engineer into APC", "engineer", engineers[0].ID, "apc", apcs[0].ID)
	return conn.Send(ipc.TypeEnterTransport, ipc.EnterTransportCommand{
		ActorID:     uint32(engineers[0].ID),
		TransportID: uint32(apcs[0].ID),
	})
}

// memoryMap fetches, allocating on first touch, a typed table in engine memory.
// Backs the per-actor throttle/tracking maps.
func memoryMap[K comparable, V any](memory map[string]any, key string) map[K]V {
	m, _ := memory[key].(map[K]V)
	if m == nil {
		m = make(map[K]V)
		memory[key] = m
	}
	return m
}

// nearestTo returns the unit closest to (x, y) and the distance.
func nearestTo(units []model.Unit, x, y int) (model.Unit, float64) {
	best := units[0]
	bestDist := math.MaxFloat64
	for _, u := range units {
		dx := float64(u.X - x)
		dy := float64(u.Y - y)
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			best = u
		}
	}
	return best, math.Sqrt(bestDist)
}

// Roomy enough to tolerate an APC stalling short of the building; the engineer
// walks the rest.
const unloadNearTargetCells = 10

// apcProgressEntry tracks an APC's position between ticks so we can detect
// a stall (APC not moving despite a pending move order) and unload anyway.
type apcProgressEntry struct {
	LastTick int
	LastX    int
	LastY    int
}

// apcExploreEntry pins an APC's exploration waypoint so it arrives somewhere
// rather than re-picking every tick. Idx rotates round-robin: a farthest-first
// heuristic oscillates between opposite corners and never sees mid-edges.
type apcExploreEntry struct {
	X          int
	Y          int
	AssignedAt int
	Idx        int
}

// apcStallTicks is how long an APC must hold one tile before we unload in
// place. ~16s at 25 Hz: OpenRA can take seconds to start pathing under load,
// and a premature unload strands the engineer at base on foot.
const apcStallTicks = 400
const apcExploreTTL = 600         // re-pick exploration target after this many ticks
const apcExploreArriveRadius = 12 // how close "arrived" counts for exploration

// apcScoutStallTicks is shorter than apcStallTicks: re-picking a waypoint is
// cheap, while unloading a scout dumps a lone engineer into fog.
const apcScoutStallTicks = 200

func getAPCProgress(memory map[string]any) map[int]apcProgressEntry {
	return memoryMap[int, apcProgressEntry](memory, "apcDeliveryProgress")
}

func getAPCExploreTargets(memory map[string]any) map[int]apcExploreEntry {
	return memoryMap[int, apcExploreEntry](memory, "apcExploreTarget")
}

// apcIsStalled also updates the tracked position as a side effect.
func apcIsStalled(memory map[string]any, u model.Unit, tick int) bool {
	return apcStalledFor(memory, u, tick, apcStallTicks)
}

// apcScoutStalled shares tracking with apcIsStalled — one source of truth for
// whether an APC is making progress.
func apcScoutStalled(memory map[string]any, u model.Unit, tick int) bool {
	return apcStalledFor(memory, u, tick, apcScoutStallTicks)
}

func apcStalledFor(memory map[string]any, u model.Unit, tick, threshold int) bool {
	return actorStalledAt(memory, "apcDeliveryProgress", u, tick, threshold)
}

// actorStalledAt tells a Move-issuing action that its destination is
// unreachable (chokepoint, terrain lock) so it advances instead of retrying
// forever. memKey keeps callers from sharing tracking unintentionally.
func actorStalledAt(memory map[string]any, memKey string, u model.Unit, tick, threshold int) bool {
	m := memoryMap[int, apcProgressEntry](memory, memKey)
	entry, ok := m[u.ID]
	if !ok || entry.LastX != u.X || entry.LastY != u.Y {
		m[u.ID] = apcProgressEntry{LastTick: tick, LastX: u.X, LastY: u.Y}
		return false
	}
	return tick-entry.LastTick >= threshold
}

// scoutStallTicks mirrors apcScoutStallTicks: tolerate path congestion, give
// up on an unreachable waypoint within ~8 seconds.
const scoutStallTicks = 200

// scoutStalled uses a separate memory slot from APC tracking so the two don't
// collide.
func scoutStalled(memory map[string]any, u model.Unit, tick int) bool {
	return actorStalledAt(memory, "scoutProgress", u, tick, scoutStallTicks)
}

// clearAPCTracking resets per-APC state after unload, including the cargo
// intent tag so the next load can re-tag the APC for a different mission.
func clearAPCTracking(memory map[string]any, id int) {
	delete(getAPCProgress(memory), id)
	delete(getAPCExploreTargets(memory), id)
	delete(getAPCMoveState(memory), id)
	ClearAPCCargoIntent(memory, id)
}

// apcMoveEntry throttles Move orders. OpenRA cancels the current activity on
// every fresh Move, so resending the same destination pins the APC in place.
type apcMoveEntry struct {
	Tick int
	X    int
	Y    int
}

// apcMoveResend — past this, assume the prior order was overridden (blocked,
// attacked) and resend.
const apcMoveResend = 60

func getAPCMoveState(memory map[string]any) map[int]apcMoveEntry {
	return memoryMap[int, apcMoveEntry](memory, "apcMoveSent")
}

// sendAPCMove is a no-op while a recent identical order is still in flight.
func sendAPCMove(env RuleEnv, conn *ipc.Connection, actorID, x, y int) error {
	state := getAPCMoveState(env.Memory)
	if prev, ok := state[actorID]; ok && prev.X == x && prev.Y == y && env.State.Tick-prev.Tick < apcMoveResend {
		return nil
	}
	state[actorID] = apcMoveEntry{Tick: env.State.Tick, X: x, Y: y}
	return conn.Send(ipc.TypeMove, ipc.MoveCommand{
		ActorID: uint32(actorID),
		X:       x,
		Y:       y,
	})
}

func ActionUnloadAPCNearTarget(env RuleEnv, conn *ipc.Connection) error {
	apcs := env.IdleEngineerLoadedAPCs()
	if len(apcs) == 0 {
		return nil
	}

	target := env.NearestCapturable()
	if target != nil {
		best, dist := nearestTo(apcs, target.X, target.Y)
		if dist < unloadNearTargetCells || apcIsStalled(env.Memory, best, env.State.Tick) {
			slog.Debug("unloading APC near target", "apc", best.ID, "target", target.ID, "dist", dist)
			clearAPCTracking(env.Memory, best.ID)
			return conn.Send(ipc.TypeUnload, ipc.UnloadCommand{ActorID: uint32(best.ID)})
		}
		slog.Debug("moving APC toward target", "apc", best.ID, "target", target.ID, "dist", dist)
		return sendAPCMove(env, conn, best.ID, target.X, target.Y)
	}

	// No visible capturable — scout with the loaded APC until one appears.
	waypoints := generateWaypoints(env.State.MapWidth, env.State.MapHeight, env.Terrain)
	if len(waypoints) == 0 {
		return nil
	}

	best := apcs[0]
	targets := getAPCExploreTargets(env.Memory)
	entry, ok := targets[best.ID]

	entry = advanceAPCPatrol(env, best, entry, ok, waypoints)
	targets[best.ID] = entry

	slog.Debug("scouting with loaded APC", "apc", best.ID, "x", entry.X, "y", entry.Y, "idx", entry.Idx)
	return sendAPCMove(env, conn, best.ID, entry.X, entry.Y)
}

// advanceAPCPatrol mirrors ActionScoutPatrol's round-robin rotation, advancing
// on arrival, stall, or TTL expiry. Farthest-first oscillates between opposite
// corners and leaves map-edge bases permanently unsighted.
func advanceAPCPatrol(env RuleEnv, apc model.Unit, prev apcExploreEntry, assigned bool, waypoints [][2]int) apcExploreEntry {
	forceAdvance := false
	if assigned {
		if env.State.Tick-prev.AssignedAt > apcExploreTTL {
			forceAdvance = true
		}
		if apcScoutStalled(env.Memory, apc, env.State.Tick) {
			forceAdvance = true
			delete(getAPCProgress(env.Memory), apc.ID)
		}
	}
	idx := nextPatrolIdx(apc.X, apc.Y, prev.X, prev.Y, prev.Idx, len(waypoints), apcExploreArriveRadius, assigned, forceAdvance)
	if !assigned {
		idx = takePatrolPoolIdx(env.Memory, "apcPatrolIdx", len(waypoints))
	}
	// Preserve AssignedAt on an unchanged index so the TTL measures dwell time
	// on the waypoint, not time since this last ran.
	assignedAt := env.State.Tick
	if assigned && idx == prev.Idx {
		assignedAt = prev.AssignedAt
	}
	return apcExploreEntry{
		X:          waypoints[idx%len(waypoints)][0],
		Y:          waypoints[idx%len(waypoints)][1],
		AssignedAt: assignedAt,
		Idx:        idx,
	}
}

// ActionLoadCombatInfantry loads one idle combat infantry per tick, skipping
// squad-assigned units so attack squads aren't drained.
func ActionLoadCombatInfantry(env RuleEnv, conn *ipc.Connection) error {
	apcs := env.IdleEmptyAPCs()
	if len(apcs) == 0 {
		return nil
	}
	assigned := squadUnitIDSet(env.Memory)
	for _, u := range env.IdleCombatInfantry() {
		if assigned[u.ID] {
			continue
		}
		// The engineer tag is sticky. Both load actions pick the same first idle
		// APC on a tick, engineer first by priority; an APC an engineer claimed
		// stays a capture mission even if combat infantry also piles in.
		intent := GetAPCCargoIntent(env.Memory)
		if intent[apcs[0].ID] != apcIntentEngineer {
			intent[apcs[0].ID] = apcIntentCombat
		}
		slog.Debug("loading combat infantry into APC", "infantry", u.ID, "apc", apcs[0].ID)
		return conn.Send(ipc.TypeEnterTransport, ipc.EnterTransportCommand{
			ActorID:     uint32(u.ID),
			TransportID: uint32(apcs[0].ID),
		})
	}
	return nil
}

// ActionDeliverAssaultAPC drives loaded APCs at the nearest known enemy base,
// ignoring water-based intel since APCs are ground units. With no land target
// it explores instead, rather than idling at base for a building sighting that
// may never come.
func ActionDeliverAssaultAPC(env RuleEnv, conn *ipc.Connection) error {
	apcs := env.IdleCombatLoadedAPCs()
	if len(apcs) == 0 {
		return nil
	}

	tx, ty := 0, 0
	hasTarget := false
	if base := env.NearestEnemyBase(); base != nil && env.IsLandAt(base.X, base.Y) {
		tx, ty = base.X, base.Y
		hasTarget = true
	} else if enemy := env.NearestEnemy(); enemy != nil && env.IsLandAt(enemy.X, enemy.Y) {
		tx, ty = enemy.X, enemy.Y
		hasTarget = true
	}

	if hasTarget {
		best, dist := nearestTo(apcs, tx, ty)
		if dist < 7 {
			slog.Debug("unloading assault APC near target", "apc", best.ID, "dist", dist)
			clearAPCTracking(env.Memory, best.ID)
			return conn.Send(ipc.TypeUnload, ipc.UnloadCommand{ActorID: uint32(best.ID)})
		}
		slog.Debug("moving assault APC toward target", "apc", best.ID, "dist", dist, "x", tx, "y", ty)
		return sendAPCMove(env, conn, best.ID, tx, ty)
	}

	// Scout until a sighting lets the branch above take over.
	waypoints := generateWaypoints(env.State.MapWidth, env.State.MapHeight, env.Terrain)
	if len(waypoints) == 0 {
		return nil
	}
	best := apcs[0]
	targets := getAPCExploreTargets(env.Memory)
	entry, ok := targets[best.ID]
	entry = advanceAPCPatrol(env, best, entry, ok, waypoints)
	targets[best.ID] = entry

	slog.Debug("scouting with assault APC", "apc", best.ID, "x", entry.X, "y", entry.Y, "idx", entry.Idx)
	return sendAPCMove(env, conn, best.ID, entry.X, entry.Y)
}

func ActionNavalAttackEnemy(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	idle := env.IdleNavalUnits()
	if len(idle) == 0 {
		return nil
	}
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Debug("naval attacking enemy", "count", len(ids), "target", enemy.ID)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        enemy.X,
		Y:        enemy.Y,
	})
}

// --- Capped attack group factories ---
// Hold back reserves by capping units per order. Superseded by squad actions in
// compiled doctrines; the seed rule set still uses them.

func GroundAttackGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		enemy := env.NearestEnemy()
		if enemy == nil {
			return nil
		}
		idle := env.IdleGroundUnits()
		if len(idle) == 0 {
			return nil
		}
		n := min(maxUnits, len(idle))
		ids := make([]uint32, n)
		for i := range n {
			ids[i] = uint32(idle[i].ID)
		}
		slog.Debug("attack-moving ground group", "count", n, "total_idle", len(idle), "target", enemy.ID)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        enemy.X,
			Y:        enemy.Y,
		})
	}
}

func GroundAttackKnownBaseGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		base := env.NearestEnemyBase()
		if base == nil {
			return nil
		}
		idle := env.IdleGroundUnits()
		if len(idle) == 0 {
			return nil
		}
		n := min(maxUnits, len(idle))
		ids := make([]uint32, n)
		for i := range n {
			ids[i] = uint32(idle[i].ID)
		}
		slog.Debug("ground attacking known base (group)", "count", n, "total_idle", len(idle), "owner", base.Owner, "x", base.X, "y", base.Y)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        base.X,
			Y:        base.Y,
		})
	}
}

func AirAttackGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		enemy := env.NearestEnemy()
		if enemy == nil {
			return nil
		}
		aircraft := env.IdleCombatAircraft()
		if len(aircraft) == 0 {
			return nil
		}
		n := min(maxUnits, len(aircraft))
		for i := range n {
			u := aircraft[i]
			slog.Debug("air attack enemy (group)", "aircraft", u.ID, "target", enemy.ID)
			if err := conn.Send(ipc.TypeAttack, ipc.AttackCommand{
				ActorID:  uint32(u.ID),
				TargetID: uint32(enemy.ID),
			}); err != nil {
				return err
			}
		}
		return nil
	}
}

func AirAttackKnownBaseGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		base := env.NearestEnemyBase()
		if base == nil {
			return nil
		}
		aircraft := env.IdleCombatAircraft()
		if len(aircraft) == 0 {
			return nil
		}
		n := min(maxUnits, len(aircraft))
		ids := make([]uint32, n)
		for i := range n {
			ids[i] = uint32(aircraft[i].ID)
		}
		slog.Debug("air attacking known base (group)", "count", n, "total_idle", len(aircraft), "owner", base.Owner, "x", base.X, "y", base.Y)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        base.X,
			Y:        base.Y,
		})
	}
}

// effectKey marks work the order stream cannot show. The engine infers effect
// by counting envelopes on the wire, so the few actions that only mutate memory
// would otherwise look like they returned early.
const effectKey = "ruleDidWork"

// markEffect records that the running action changed something.
func markEffect(env RuleEnv) {
	if env.Memory != nil {
		env.Memory[effectKey] = true
	}
}

// --- Squad action factories ---

// FormSquad only assigns unit IDs; it issues no orders. Formation and action
// are separate rules so the compiler can give each its own priority and
// condition.
func FormSquad(name, domain string, size int, role string) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		var pool []model.Unit
		switch domain {
		case "ground":
			pool = env.UnassignedIdleGround()
		case "air":
			pool = env.UnassignedIdleAir()
		case "naval":
			pool = env.UnassignedIdleNaval()
		default:
			pool = env.UnassignedIdleGround()
		}

		squads := getSquads(env.Memory)
		if sq, ok := squads[name]; ok && len(sq.UnitIDs) > 0 {
			// Reinforcement: top up an existing under-strength squad.
			if len(sq.UnitIDs) >= sq.TargetSize || len(pool) == 0 {
				return nil
			}
			need := sq.TargetSize - len(sq.UnitIDs)
			add := min(need, len(pool))
			for i := range add {
				sq.UnitIDs = append(sq.UnitIDs, pool[i].ID)
			}
			env.Memory["squads"] = squads
			markEffect(env)
			slog.Info("squad reinforced", "name", name, "added", add, "size", len(sq.UnitIDs), "target", sq.TargetSize)
			return nil
		}

		// Take what the pool has. Whether that is enough to be worth forming is
		// the rule's question, not this function's — form-ground-attack asks for
		// 60% of the group size and tops up — so insisting on the full target
		// here silently forms nothing.
		if len(pool) == 0 {
			return nil
		}
		take := min(size, len(pool))
		ids := make([]int, take)
		for i := range take {
			ids[i] = pool[i].ID
		}
		squads[name] = &Squad{
			Name:       name,
			Domain:     domain,
			UnitIDs:    ids,
			Role:       role,
			TargetSize: size,
		}
		env.Memory["squads"] = squads
		markEffect(env)
		slog.Info("squad formed", "name", name, "domain", domain, "role", role,
			"size", take, "target", size)
		return nil
	}
}

// huntBaseState tracks which radial position a squad is cycling through
// when hunting around an enemy base. Stored in memory per squad name.
type huntBaseState struct {
	BaseX, BaseY int
	Step         int
}

// huntOffset maps a hunt step to an offset from the base centroid: step 0 is
// the centroid, then two rings of 8 compass points at radius R and 2R. Sweeping
// inward-first catches buildings just inside fog before widening to outliers.
// huntMaxStep is where the hunt wraps back to its first ring.
//
// The rings have to be able to span the map. A razed base leaves its last
// building wherever it stood, which is not necessarily beside the centroid its
// sightings averaged out to — game 105 reduced the enemy to one outlying
// barracks and then circled the empty base site, because two rings of four
// cells could not reach it. Widening is safe: squad-attack outranks the base
// attack the moment anything is visible, so a growing search only runs while
// there is nothing in sight, and finding something ends it.
func huntMaxStep(mapDim, radius int) int {
	if radius < 1 {
		radius = 1
	}
	// Round the ring count up: truncating leaves the outermost ring short of
	// the half-map it is meant to cover.
	rings := (mapDim/2 + radius - 1) / radius
	return 8 * max(2, rings)
}

func huntOffset(step, radius int) (int, int) {
	if step <= 0 {
		return 0, 0
	}
	idx := (step - 1) % 8       // which of 8 compass points (0-7)
	ring := (step-1)/8 + 1      // which ring: 1 for steps 1-8, 2 for 9-16
	r := float64(radius * ring) // inner ring = R, outer ring = 2R
	angle := float64(idx) * 2 * math.Pi / 8
	return int(r * math.Cos(angle)), int(r * math.Sin(angle))
}

// squadAttackState records target commitment so rally-then-attack demands
// clumping only on the initial deploy. Re-checking mid-fight oscillates:
// attack, spread, regroup, attack.
type squadAttackState struct {
	TargetX, TargetY int
	Attacking        bool // false = still regrouping to centroid
	LastTick         int
}

const (
	// Long enough to finish a typical engagement, short enough that a new
	// target still gets a fresh assembly.
	squadAttackCommitTTL = 2000
	// Clumped means 80% of members within this many cells of the centroid.
	squadRallyRadius = 8
)

func SquadAttackMove(name string) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		enemy := env.BestGroundTarget()
		if enemy == nil {
			enemy = env.NearestEnemy()
		}
		if enemy == nil {
			return nil
		}
		ids := squadIdleActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}

		// Rally before committing: AttackMove to our own centroid pulls
		// stragglers up and halts the leaders, so the squad arrives together
		// rather than being fed in piecemeal.
		state := memoryMap[string, squadAttackState](env.Memory, "squadAttackState")
		prev, hasPrev := state[name]
		targetChanged := !hasPrev || prev.TargetX != enemy.X || prev.TargetY != enemy.Y
		commitStale := hasPrev && env.State.Tick-prev.LastTick > squadAttackCommitTTL
		if targetChanged || commitStale || !prev.Attacking {
			if env.SquadClumped(name, squadRallyRadius) {
				state[name] = squadAttackState{TargetX: enemy.X, TargetY: enemy.Y, Attacking: true, LastTick: env.State.Tick}
			} else {
				cx, cy, ok := squadCentroid(env, name)
				if ok {
					state[name] = squadAttackState{TargetX: enemy.X, TargetY: enemy.Y, Attacking: false, LastTick: env.State.Tick}
					slog.Debug("squad rallying before attack", "squad", name, "count", len(ids), "target", enemy.ID, "centroid_x", cx, "centroid_y", cy)
					return sendAttackMove(env, conn, ids, cx, cy)
				}
				// No centroid — fall through to direct attack.
				state[name] = squadAttackState{TargetX: enemy.X, TargetY: enemy.Y, Attacking: true, LastTick: env.State.Tick}
			}
		} else {
			prev.LastTick = env.State.Tick
			state[name] = prev
		}

		// Route around a hot defense corridor rather than attack-moving through
		// it. The waypoint helper gates on threat itself, so a clear path
		// declines and we fall through to direct routing.
		tx, ty := enemy.X, enemy.Y
		if wx, wy, ok := groundApproachWaypointFor(env, name, enemy.X, enemy.Y); ok {
			tx, ty = wx, wy
			slog.Debug("squad routing via waypoint", "squad", name, "wp_x", wx, "wp_y", wy, "target", enemy.ID)
		}

		slog.Debug("squad attack-move", "squad", name, "count", len(ids), "target", enemy.ID, "x", tx, "y", ty)
		return sendAttackMove(env, conn, ids, tx, ty)
	}
}

// groundApproachWaypointFor returns a threat-aware staging point if the squad
// is far from `dest` AND a waypoint would actually shift the approach. Returns
// false when the squad is already engaging or no useful waypoint exists.
func groundApproachWaypointFor(env RuleEnv, name string, destX, destY int) (int, int, bool) {
	sqCX, sqCY, ok := squadCentroid(env, name)
	if !ok {
		return 0, 0, false
	}
	const engageDistSq = 30 * 30
	dx, dy := sqCX-destX, sqCY-destY
	if dx*dx+dy*dy < engageDistSq {
		return 0, 0, false
	}
	wx, wy, has := env.BestApproachAxis(destX, destY)
	if !has {
		return 0, 0, false
	}
	const waypointRadiusSq = 20 * 20
	wdx, wdy := sqCX-wx, sqCY-wy
	if wdx*wdx+wdy*wdy < waypointRadiusSq {
		return 0, 0, false
	}
	return wx, wy, true
}

// attackOrderEntry throttles repeat Attack orders. OpenRA treats each as
// cancel-and-restart, so re-issuing pins units in place unable to close and
// fire — the same failure mode as Move and Capture.
type attackOrderEntry struct {
	Tick     int
	TargetID int
}

const attackOrderResend = 60

// sendAttack suppresses identical re-issues within attackOrderResend ticks.
func sendAttack(env RuleEnv, conn *ipc.Connection, actorID, targetID uint32) error {
	state := memoryMap[int, attackOrderEntry](env.Memory, "attackOrderSent")
	if prev, ok := state[int(actorID)]; ok && prev.TargetID == int(targetID) && env.State.Tick-prev.Tick < attackOrderResend {
		return nil
	}
	state[int(actorID)] = attackOrderEntry{Tick: env.State.Tick, TargetID: int(targetID)}
	return conn.Send(ipc.TypeAttack, ipc.AttackCommand{
		ActorID:  actorID,
		TargetID: targetID,
	})
}

// attackMoveEntry tracks the last TypeAttackMove order issued per actor.
type attackMoveEntry struct {
	Tick int
	X, Y int
}

const attackMoveResend = 60

// sendAttackMove batches an AttackMove across actorIDs, skipping any actor
// with an identical in-flight order. Each fresh command cancels the current
// path, so an unthrottled squad action stalls its units mid-map.
func sendAttackMove(env RuleEnv, conn *ipc.Connection, actorIDs []uint32, x, y int) error {
	state := memoryMap[int, attackMoveEntry](env.Memory, "attackMoveSent")
	var toSend []uint32
	for _, id := range actorIDs {
		prev, ok := state[int(id)]
		if ok && prev.X == x && prev.Y == y && env.State.Tick-prev.Tick < attackMoveResend {
			continue
		}
		state[int(id)] = attackMoveEntry{Tick: env.State.Tick, X: x, Y: y}
		toSend = append(toSend, id)
	}
	if len(toSend) == 0 {
		return nil
	}
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: toSend, X: x, Y: y,
	})
}

func SquadAttackKnownBase(name string, aggression float64) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		base := env.NearestEnemyBase()
		if base == nil {
			return nil
		}
		ids := squadIdleActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}

		// Base attacks are the worst dispersal case — the walk is long enough
		// that tanks and dogs arrive and die before the rifles catch up.
		aState := memoryMap[string, squadAttackState](env.Memory, "squadAttackState")
		prev, hasPrev := aState[name]
		targetChanged := !hasPrev || prev.TargetX != base.X || prev.TargetY != base.Y
		commitStale := hasPrev && env.State.Tick-prev.LastTick > squadAttackCommitTTL
		if targetChanged || commitStale || !prev.Attacking {
			if env.SquadClumped(name, squadRallyRadius) {
				aState[name] = squadAttackState{TargetX: base.X, TargetY: base.Y, Attacking: true, LastTick: env.State.Tick}
			} else {
				cx, cy, ok := squadCentroid(env, name)
				if ok {
					aState[name] = squadAttackState{TargetX: base.X, TargetY: base.Y, Attacking: false, LastTick: env.State.Tick}
					slog.Debug("squad rallying before base-attack", "squad", name, "count", len(ids), "base_x", base.X, "base_y", base.Y, "centroid_x", cx, "centroid_y", cy)
					return sendAttackMove(env, conn, ids, cx, cy)
				}
				aState[name] = squadAttackState{TargetX: base.X, TargetY: base.Y, Attacking: true, LastTick: env.State.Tick}
			}
		} else {
			prev.LastTick = env.State.Tick
			aState[name] = prev
		}

		memKey := "huntBase:" + name
		state, _ := env.Memory[memKey].(*huntBaseState)
		if state == nil {
			state = &huntBaseState{}
		}

		if state.BaseX != base.X || state.BaseY != base.Y {
			state.BaseX = base.X
			state.BaseY = base.Y
			state.Step = 0
		}

		tx, ty := base.X, base.Y

		// Enter via a zone that skirts remembered defenses; once the squad is
		// there, later steps run at the base centroid as normal.
		if state.Step == 0 {
			if wx, wy, ok := env.BestApproachAxis(base.X, base.Y); ok {
				sqCX, sqCY, have := squadCentroid(env, name)
				if have {
					const waypointRadiusSq = 20 * 20
					dx, dy := sqCX-wx, sqCY-wy
					if dx*dx+dy*dy > waypointRadiusSq {
						tx, ty = wx, wy
						slog.Debug("squad routing via approach waypoint",
							"squad", name, "wp_x", wx, "wp_y", wy, "base_x", base.X, "base_y", base.Y)
					}
				}
			}
		}

		mapDim := max(env.State.MapWidth, env.State.MapHeight)
		baseRadius := mapDim / 16
		scale := 0.25 + aggression*1.25
		radius := int(float64(baseRadius) * scale)
		if radius < 1 {
			radius = 1
		}
		maxStep := huntMaxStep(mapDim, radius)

		if state.Step > 0 {
			dx, dy := huntOffset(state.Step, radius)
			tx = base.X + dx
			ty = base.Y + dy

			tx = max(0, min(tx, env.State.MapWidth-1))
			ty = max(0, min(ty, env.State.MapHeight-1))

			squads := getSquads(env.Memory)
			sq := squads[name]
			if sq != nil && sq.Domain != "air" && env.Terrain != nil {
				t := env.Terrain.AtMapPos(tx, ty)
				if t != model.Land && t != model.Bridge {
					tx, ty = base.X, base.Y // fallback to centroid
				}
			}
		}

		// Suppress the step advance along with the send: otherwise the 16-step
		// hunt rotates once per tick and no step is ever reached.
		if !attackMoveHasFreshTarget(env, ids, tx, ty) {
			env.Memory[memKey] = state
			return nil
		}

		slog.Debug("squad attacking known base", "squad", name, "count", len(ids),
			"owner", base.Owner, "step", state.Step, "x", tx, "y", ty)

		// Wraps to 1, not 0 — the centroid is only worth the initial approach.
		if state.Step >= maxStep {
			state.Step = 1
		} else {
			state.Step++
		}
		env.Memory[memKey] = state

		return sendAttackMove(env, conn, ids, tx, ty)
	}
}

// attackMoveHasFreshTarget reports whether a send to (x,y) would reach any
// actor, so callers advance internal state only when the order will land.
func attackMoveHasFreshTarget(env RuleEnv, ids []uint32, x, y int) bool {
	state := memoryMap[int, attackMoveEntry](env.Memory, "attackMoveSent")
	for _, id := range ids {
		prev, ok := state[int(id)]
		if !ok || prev.X != x || prev.Y != y || env.State.Tick-prev.Tick >= attackMoveResend {
			return true
		}
	}
	return false
}

func SquadDefend(name string) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		enemy := env.BestGroundTarget()
		if enemy == nil {
			enemy = env.NearestEnemy()
		}
		if enemy == nil {
			return nil
		}
		ids := squadIdleActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}
		slog.Debug("squad defending", "squad", name, "count", len(ids), "target", enemy.ID)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        enemy.X,
			Y:        enemy.Y,
		})
	}
}

// SquadGuardHarvesters sends a squad to a threatened harvester.
//
// Every other defensive action gates on the base being attacked, so a harvester
// shot at a distant ore patch summoned nobody and the only response was to run
// it home and abandon the ore.
//
// Attack-move, not move: the point is to engage what is shooting it.
func SquadGuardHarvesters(name string, dangerPct float64) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		threatened := env.HarvestersInDanger(dangerPct)
		if len(threatened) == 0 {
			return nil
		}
		ids := squadCommittableActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}

		// Nearest first, so a guard doesn't cross the map past a closer
		// harvester in the same trouble.
		target := threatened[0]
		if cx, cy, ok := squadCentroid(env, name); ok {
			best := math.MaxFloat64
			for _, h := range threatened {
				dx, dy := float64(h.X-cx), float64(h.Y-cy)
				if d := dx*dx + dy*dy; d < best {
					best, target = d, h
				}
			}
		}

		// Re-issue only on a target change or a stale order; anything else
		// cancels the in-flight path and the squad never arrives.
		type guardOrder struct {
			Target int
			Tick   int
		}
		orders := memoryMap[string, guardOrder](env.Memory, "squadGuardOrder")
		if prev, ok := orders[name]; ok &&
			prev.Target == target.ID && env.State.Tick-prev.Tick < guardResendTicks {
			return nil
		}
		orders[name] = guardOrder{Target: target.ID, Tick: env.State.Tick}

		slog.Debug("squad guarding harvester",
			"squad", name, "count", len(ids), "harvester", target.ID)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        target.X,
			Y:        target.Y,
		})
	}
}

// squadCentroid returns false for an empty or unknown squad.
func squadCentroid(env RuleEnv, name string) (int, int, bool) {
	squads := getSquads(env.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return 0, 0, false
	}
	ids := make(map[int]bool, len(sq.UnitIDs))
	for _, id := range sq.UnitIDs {
		ids[id] = true
	}
	sumX, sumY, count := 0, 0, 0
	for _, u := range env.State.Units {
		if ids[u.ID] {
			sumX += u.X
			sumY += u.Y
			count++
		}
	}
	if count == 0 {
		return 0, 0, false
	}
	return sumX / count, sumY / count, true
}

// squadCommittableActorIDs includes non-idle members. A squad already moving
// toward one threatened harvester isn't idle, so waiting for idleness means
// never answering the next raid — and harassment is continuous.
func squadCommittableActorIDs(env RuleEnv, name string) []uint32 {
	squads := getSquads(env.Memory)
	sq, ok := squads[name]
	if !ok {
		return nil
	}
	retreating := getRetreatingUnits(env.Memory)
	var ids []uint32
	for _, id := range sq.UnitIDs {
		if _, isRetreating := retreating[id]; !isRetreating {
			ids = append(ids, uint32(id))
		}
	}
	return ids
}

func squadIdleActorIDs(env RuleEnv, name string) []uint32 {
	squads := getSquads(env.Memory)
	sq, ok := squads[name]
	if !ok {
		return nil
	}
	idleSet := make(map[int]bool)
	for _, u := range env.State.Units {
		if u.Idle {
			idleSet[u.ID] = true
		}
	}
	retreating := getRetreatingUnits(env.Memory)
	held := memoryMap[int, int](env.Memory, "disengagedUntil")
	var ids []uint32
	for _, id := range sq.UnitIDs {
		_, isRetreating := retreating[id]
		// A unit that has just withdrawn is not available to the attacking
		// rules until it has had time to regroup. squadCommittableActorIDs is
		// deliberately not filtered this way: a withdrawal must still be able
		// to command units it has already pulled back.
		if until, ok := held[id]; ok {
			if env.State.Tick < until {
				continue
			}
			delete(held, id)
		}
		if idleSet[id] && !isRetreating {
			ids = append(ids, uint32(id))
		}
	}
	return ids
}

// --- Micro action factories ---

// RetreatDamagedUnits uses Move, not AttackMove — a retreating unit that stops
// to fight defeats the point. Vehicles go to the service depot to auto-repair,
// everything else behind base defenses. Retreating units are marked in memory
// so focus-fire and squad-attack skip them.
func RetreatDamagedUnits(hpThreshold float64) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		units := env.DamagedCombatUnits(hpThreshold)
		if len(units) == 0 {
			return nil
		}
		retreating := getRetreatingUnits(env.Memory)
		if retreating == nil {
			retreating = make(map[int]int)
		}

		depot := env.ServiceDepot()
		airfield := env.Airfield()
		centX, centY := env.BuildingCentroid()

		for _, u := range units {
			if isInfantry(u) {
				continue // infantry can't heal — no benefit to retreating
			}
			if isAircraft(u) && airfield != nil {
				slog.Debug("retreating damaged aircraft to airfield", "id", u.ID, "type", u.Type,
					"hp_ratio", float64(u.HP)/float64(u.MaxHP), "airfield", airfield.ID)
				if err := conn.Send(ipc.TypeRepairUnit, ipc.RepairUnitCommand{
					ActorID:          uint32(u.ID),
					RepairBuildingID: uint32(airfield.ID),
				}); err != nil {
					return err
				}
			} else if depot != nil && !isAircraft(u) && !isNaval(u) {
				slog.Debug("retreating damaged unit to depot", "id", u.ID, "type", u.Type,
					"hp_ratio", float64(u.HP)/float64(u.MaxHP), "depot", depot.ID)
				if err := conn.Send(ipc.TypeRepairUnit, ipc.RepairUnitCommand{
					ActorID:          uint32(u.ID),
					RepairBuildingID: uint32(depot.ID),
				}); err != nil {
					return err
				}
			} else {
				slog.Debug("retreating damaged unit to centroid", "id", u.ID, "type", u.Type,
					"hp_ratio", float64(u.HP)/float64(u.MaxHP), "dest_x", centX, "dest_y", centY)
				if err := conn.Send(ipc.TypeMove, ipc.MoveCommand{
					ActorID: uint32(u.ID), X: centX, Y: centY,
				}); err != nil {
					return err
				}
			}
			retreating[u.ID] = env.State.Tick
		}
		env.Memory["retreatingUnits"] = retreating
		return nil
	}
}

// ClearHealedUnits returns healed, dead, or timed-out units to the combat pool.
// The timeout stops units leaking permanently when repair never completes —
// depot destroyed mid-repair, say.
func ClearHealedUnits(hpThreshold float64) ActionFunc {
	const retreatTimeout = 200 // ticks before forcing release

	return func(env RuleEnv, conn *ipc.Connection) error {
		retreating := getRetreatingUnits(env.Memory)
		if len(retreating) == 0 {
			return nil
		}
		before := len(retreating)
		aliveIDs := make(map[int]bool)
		for _, u := range env.State.Units {
			aliveIDs[u.ID] = true
			if _, ok := retreating[u.ID]; ok && u.MaxHP > 0 && float64(u.HP)/float64(u.MaxHP) >= hpThreshold {
				delete(retreating, u.ID)
				slog.Debug("unit healed, returning to duty", "id", u.ID, "type", u.Type)
			}
		}
		for id, tick := range retreating {
			if !aliveIDs[id] || (env.State.Tick-tick > retreatTimeout) {
				if aliveIDs[id] {
					slog.Debug("retreat timeout, returning to duty", "id", id, "elapsed", env.State.Tick-tick)
				}
				delete(retreating, id)
			}
		}
		env.Memory["retreatingUnits"] = retreating
		if len(retreating) < before {
			markEffect(env)
		}
		return nil
	}
}

// RecallOverextended moves idle squad members that have wandered too far
// from base back toward the building centroid.
func RecallOverextended(name string, leashPct float64) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		units := env.OverextendedSquadMembers(name, leashPct)
		if len(units) == 0 {
			return nil
		}
		centX, centY := env.BuildingCentroid()
		for _, u := range units {
			slog.Debug("recalling overextended unit", "squad", name, "id", u.ID, "dest_x", centX, "dest_y", centY)
			if err := conn.Send(ipc.TypeMove, ipc.MoveCommand{
				ActorID: uint32(u.ID), X: centX, Y: centY,
			}); err != nil {
				return err
			}
		}
		return nil
	}
}

// SquadDisengage moves idle squad members back toward base centroid when
// the local threat ratio is too high (outmatched).
func SquadDisengage(name string) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		// Committable, not idle. A squad in combat is not idle, and combat is
		// the only situation this runs in: the rule requires the squad to be
		// away from base and outnumbered. Gating on idleness meant the moment
		// it most needed to withdraw it had nobody to order. Across games 88,
		// 90 and 91 squad-disengage-ground-attack matched 155 times and ordered
		// something 10 — 145 decisions to retreat that never reached a unit,
		// while every vehicle built died: 6 delivered 6 lost in game 90, 8 and
		// 8 in game 91.
		//
		// The same mistake as guard-harvesters, fixed there on 2026-09-08 and
		// not generalised.
		ids := squadCommittableActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}
		// Re-issuing a move every tick cancels the in-flight path, so a
		// retreating squad would stand still being shot. Only re-order when the
		// destination has really changed or the order has gone stale.
		centX, centY := env.BuildingCentroid()
		type disengageOrder struct {
			X, Y, Tick int
		}
		orders := memoryMap[string, disengageOrder](env.Memory, "squadDisengageOrder")
		if prev, ok := orders[name]; ok &&
			prev.X == centX && prev.Y == centY &&
			env.State.Tick-prev.Tick < disengageResendTicks {
			return nil
		}
		orders[name] = disengageOrder{X: centX, Y: centY, Tick: env.State.Tick}

		held := memoryMap[int, int](env.Memory, "disengagedUntil")
		for _, id := range ids {
			held[int(id)] = env.State.Tick + disengageHoldTicks
		}

		for _, id := range ids {
			slog.Debug("squad disengaging", "squad", name, "unit", id, "dest_x", centX, "dest_y", centY)
			if err := conn.Send(ipc.TypeMove, ipc.MoveCommand{
				ActorID: id, X: centX, Y: centY,
			}); err != nil {
				return err
			}
		}
		return nil
	}
}

// SquadFocusFire concentrates the whole squad on one high-value target rather
// than letting each member pick its own.
func SquadFocusFire(name string) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		target := env.BestGroundTarget()
		if target == nil {
			return nil
		}
		ids := squadIdleActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			slog.Debug("squad focus fire", "squad", name, "unit", id, "target", target.ID)
			if err := sendAttack(env, conn, id, uint32(target.ID)); err != nil {
				return err
			}
		}
		return nil
	}
}

// SquadAirStrike concentrates the air squad on one high-value target.
//
// A heavy AA corridor is staged around first: without it aircraft fly straight
// through SAM clusters every time.
func SquadAirStrike(name string) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		target := env.BestAirTarget()
		if target == nil {
			return nil
		}
		ids := squadIdleActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}

		if wx, wy, ok := airApproachWaypointFor(env, name, target.X, target.Y); ok {
			slog.Debug("squad air routing via waypoint", "squad", name, "wp_x", wx, "wp_y", wy, "target", target.ID)
			return sendAttackMove(env, conn, ids, wx, wy)
		}

		for _, id := range ids {
			slog.Debug("squad air strike", "squad", name, "unit", id, "target", target.ID)
			if err := sendAttack(env, conn, id, uint32(target.ID)); err != nil {
				return err
			}
		}
		return nil
	}
}

// airApproachWaypointFor returns a low-AA staging point, or false once the
// squad has reached it so callers fall through to attacking the target.
func airApproachWaypointFor(env RuleEnv, name string, destX, destY int) (int, int, bool) {
	sqCX, sqCY, ok := squadCentroid(env, name)
	if !ok {
		return 0, 0, false
	}
	const engageDistSq = 40 * 40
	dx, dy := sqCX-destX, sqCY-destY
	if dx*dx+dy*dy < engageDistSq {
		return 0, 0, false
	}
	wx, wy, has := env.BestAirApproachAxis(destX, destY)
	if !has {
		return 0, 0, false
	}
	const waypointRadiusSq = 20 * 20
	wdx, wdy := sqCX-wx, sqCY-wy
	if wdx*wdx+wdy*wdy < waypointRadiusSq {
		return 0, 0, false
	}
	return wx, wy, true
}

// harvesterFleeEntry throttles flee orders. Each re-issue invalidates the
// server's pathfinder and pins the harvester where it stands, so fresh orders
// go out only on a changed destination or a stale prior order.
type harvesterFleeEntry struct {
	Tick int
	X    int
	Y    int
}

// harvesterFleeResend — still in danger this long after a flee means the order
// was cancelled or the harvester is stuck.
const harvesterFleeResend = 100

func getHarvesterFleeState(memory map[string]any) map[int]harvesterFleeEntry {
	return memoryMap[int, harvesterFleeEntry](memory, "harvesterFleeing")
}

// CountFleeingHarvesters lets the agent package detect sustained harassment
// without depending on the unexported entry struct. Reflection, rather than a
// type assertion, so tests can populate the map with their own value type.
func CountFleeingHarvesters(memory map[string]any) int {
	v, ok := memory["harvesterFleeing"]
	if !ok {
		return 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return 0
	}
	return rv.Len()
}

func FleeHarvesters(dangerPct float64) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		harvesters := env.HarvestersInDanger(dangerPct)

		// Prune before the early return. This action only runs while something is
		// in danger, so it is the only chance to clear the map at all.
		state := getHarvesterFleeState(env.Memory)
		inDanger := make(map[int]bool, len(harvesters))
		for _, u := range harvesters {
			inDanger[u.ID] = true
		}
		for id := range state {
			if !inDanger[id] {
				delete(state, id)
			}
		}

		if len(harvesters) == 0 {
			return nil
		}
		var refineries []model.Building
		for _, b := range env.State.Buildings {
			if matchesType(b.Type, Refinery) {
				refineries = append(refineries, b)
			}
		}
		fallbackX, fallbackY := 0, 0
		if len(env.State.Buildings) > 0 {
			sumX, sumY := 0, 0
			for _, b := range env.State.Buildings {
				sumX += b.X
				sumY += b.Y
			}
			fallbackX = sumX / len(env.State.Buildings)
			fallbackY = sumY / len(env.State.Buildings)
		}

		for _, u := range harvesters {
			tx, ty := fallbackX, fallbackY
			if len(refineries) > 0 {
				bestDist := math.MaxFloat64
				for _, r := range refineries {
					dx := float64(u.X - r.X)
					dy := float64(u.Y - r.Y)
					d := dx*dx + dy*dy
					if d < bestDist {
						bestDist = d
						tx, ty = r.X, r.Y
					}
				}
			}
			if prev, ok := state[u.ID]; ok && prev.X == tx && prev.Y == ty && env.State.Tick-prev.Tick < harvesterFleeResend {
				continue
			}
			state[u.ID] = harvesterFleeEntry{Tick: env.State.Tick, X: tx, Y: ty}
			slog.Debug("fleeing harvester", "id", u.ID, "dest_x", tx, "dest_y", ty)
			if err := conn.Send(ipc.TypeMove, ipc.MoveCommand{
				ActorID: uint32(u.ID),
				X:       tx,
				Y:       ty,
			}); err != nil {
				return err
			}
		}
		return nil
	}
}

func NavalAttackGroup(maxUnits int) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		enemy := env.NearestEnemy()
		if enemy == nil {
			return nil
		}
		idle := env.IdleNavalUnits()
		if len(idle) == 0 {
			return nil
		}
		n := min(maxUnits, len(idle))
		ids := make([]uint32, n)
		for i := range n {
			ids[i] = uint32(idle[i].ID)
		}
		slog.Debug("naval attacking enemy (group)", "count", n, "total_idle", len(idle), "target", enemy.ID)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        enemy.X,
			Y:        enemy.Y,
		})
	}
}

// unblockEgressResend — long enough for a normal move to complete, short
// enough to retry promptly when the blocker is pinned and can't leave.
const unblockEgressResend = 120

type egressEntry struct{ Tick int }

// ActionUnblockWarFactoryEgress scatters a friendly unit camped on the war
// factory exit. A vehicle stuck behind one hangs the Vehicle queue at 100% and
// every produce rule downstream of it stalls for the rest of the game.
//
// The blocker is pushed along the factory-to-centroid axis so it moves into the
// base interior rather than back across the tile it was blocking.
func ActionUnblockWarFactoryEgress(env RuleEnv, conn *ipc.Connection) error {
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
