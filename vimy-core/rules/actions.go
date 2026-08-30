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

// mcvDeployState tracks Deploy attempts per MCV so the rule can detect a
// stuck MCV (Deploy silently rejected by the engine — usually no clear 3x3
// footprint at the MCV's current position) and relocate it to a fresh spot
// instead of spamming the same failing order forever. Game 32 (337k-tick
// loss) saw 25898 deploy-mcv firings after CY was destroyed at tick 75930;
// the recovered MCVs sat exposed and got picked off, repeat (vimy-rw8).
type mcvDeployState struct {
	Attempts int
	Tick     int
}

const (
	mcvDeployCooldownTicks = 50
	mcvMaxDeployAttempts   = 3
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

	// After K failed deploys at this MCV's current spot, send it to a fresh
	// nearby land tile and reset the counter. The next time it arrives idle,
	// Deploy is retried at the new position.
	if state.Attempts >= mcvMaxDeployAttempts {
		fx, fy := mcvFallbackLocation(env)
		state.Attempts = 0
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

// mcvFallbackLocation picks a land tile near our existing infrastructure
// where a fresh MCV deploy is likely to succeed. Tries 8 compass offsets
// around the building centroid until one lands on Land; falls back to the
// raw centroid when terrain is unknown or all offsets are blocked.
func mcvFallbackLocation(env RuleEnv) (int, int) {
	cx, cy := env.BuildingCentroid()
	if env.Terrain == nil {
		return cx, cy
	}
	offsets := [8][2]int{
		{300, 0}, {-300, 0}, {0, 300}, {0, -300},
		{200, 200}, {-200, 200}, {200, -200}, {-200, -200},
	}
	for _, o := range offsets {
		tx := clampInt(cx+o[0], 0, env.State.MapWidth-1)
		ty := clampInt(cy+o[1], 0, env.State.MapHeight-1)
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

// ActionCancelStuckAircraft works around an OpenRA quirk: aircraft production
// completes but sometimes can't spawn (no free pad). Cancelling frees the queue.
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

// chokePos is an internal helper: a ranked chokepoint's map-space center plus
// its score, pre-computed so candidate scoring doesn't re-walk the grid.
type chokePos struct {
	x, y  int
	score float64
}

// defenseHint generates a scored placement hint for defense buildings.
// It evaluates 16 candidate positions around the base perimeter annulus
// (100%-150% of radius), scores each by four weighted factors, then picks
// randomly from the top 3 to balance strategic placement with unpredictability.
func defenseHint(env RuleEnv) (int, int) {
	buildings := env.State.Buildings
	if len(buildings) == 0 {
		return 0, 0
	}

	// Compute base centroid.
	sumX, sumY := 0, 0
	for _, b := range buildings {
		sumX += b.X
		sumY += b.Y
	}
	cx := sumX / len(buildings)
	cy := sumY / len(buildings)

	// Compute base radius from the furthest building.
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

	// Threat direction: unit vector toward nearest known enemy base.
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

	// High-value building positions.
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

	// Existing defense positions for spread calculation.
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

	// Nearby ranked chokepoints — bridges and land pinches within ~2× base
	// radius. Used both as seed candidates (so a choke adjacent to base can
	// actually win placement) and as a proximity bonus for annulus candidates.
	//
	// Seed positions are offset from the zone center back toward our base by
	// about half a zone so the defense ends up flanking the choke, not
	// sitting *on* the crossing — placing a pillbox on the only bridge tile
	// would strand our own harvesters and reinforcements.
	// vimy harvester-defense (next-up after vimy-b14): when harvesters are
	// currently fleeing, the threat is on the economy rather than the front.
	// Seed extra candidates in a tight annulus around each refinery and boost
	// the protection-near-refinery score component so defenses get placed at
	// the threatened refinery rather than on the threat-toward-enemy axis.
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
			// Offset seed position back toward base centroid by half a zone
			// width, so the defense covers the choke without blocking it.
			sx, sy := zx, zy
			if d > 0 {
				offset := 0.5 * float64(env.Terrain.CellW)
				if env.Terrain.CellH < env.Terrain.CellW {
					offset = 0.5 * float64(env.Terrain.CellH)
				}
				sx = zx - int(dx/d*offset)
				sy = zy - int(dy/d*offset)
			}
			// Reject if the offset still lands on a non-land tile (bridge,
			// water, cliff). Better to skip the seed than block a crossing.
			if t := env.Terrain.AtMapPos(sx, sy); t != model.Land {
				continue
			}
			nearbyChokes = append(nearbyChokes, chokePos{x: sx, y: sy, score: c.Score})
		}
	}

	// Generate 16 candidates in the perimeter annulus (100%-150% of radius)
	// plus seed candidates at each nearby chokepoint.
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
	// Refinery perimeter seeding when harvesters are under attack. 4 candidates
	// per refinery at ~3-cell radius, offset on cardinal directions so they
	// form a small ring covering the most common raid approach angles.
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

		// Terrain filter. Exclude Bridge as well as water/cliff — placing a
		// defense on a bridge tile would physically block our own units
		// crossing it.
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

		// Score: refinery proximity, only counts during harvester emergency.
		// Distance is measured to the nearest refinery, not generic high-value
		// buildings, so the boost specifically pulls defenses to the threatened
		// economy buildings rather than the tech center / war factory cluster.
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

		// Score: chokepoint proximity (weight 0.20). Rewards candidates near a
		// ranked chokepoint weighted by the choke's own score — a bridge on
		// the enemy path beats an off-route land pinch.
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

		// Harvester emergency: shift weight away from "push perimeter toward
		// enemy" (threatScore + perimeterScore + chokeScore drop) and onto
		// "cover the refinery being raided" (refineryScore takes 0.50).
		// Spread is preserved at a modest weight so we don't pile all defenses
		// on one refinery wall.
		var score float64
		if harvesterEmergency {
			score = 0.50*refineryScore + 0.15*spreadScore + 0.15*threatScore + 0.10*chokeScore + 0.10*perimeterScore
		} else {
			score = 0.30*threatScore + 0.20*chokeScore + 0.15*protectionScore + 0.20*spreadScore + 0.15*perimeterScore
		}
		candidates = append(candidates, candidate{x, y, score, seed})
	}

	// Fallback: all candidates filtered out (water/cliff everywhere).
	if len(candidates) == 0 {
		return cx, cy
	}

	// Sort by score descending, pick randomly from top 3.
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

// defenseRoles lists all ground defense types eligible for the diversified
// defense producer. Order doesn't matter — selection is by lowest count.
var defenseRoles = []string{"pillbox", "camo_pillbox", "turret", "flame_tower", "tesla_coil"}

func ActionProduceDefense(env RuleEnv, conn *ipc.Connection) error {
	// Pick the buildable defense type we have the fewest of. This diversifies
	// the defense mix instead of always building the first available type.
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
	// Only pull genuinely unassigned idle units. Ground-attack squad members
	// stay with their squad — they're being held for the push, not for base
	// defense. Otherwise a lone raider near base poaches the attack squad
	// every tick and the squad never deploys (game 46: form-ground-attack
	// fired 341x but squad-attack fired 1x). Base defense is the ground-
	// defense squad's job; if that squad hasn't formed yet, it will when
	// enough unassigned units accumulate.
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

// ActionDefendCriticalBuilding pulls ALL nearby ground units — regardless
// of squad membership or idle status — to attack the nearest enemy that's
// engaging one of our critical buildings. This overrides the poach-
// prevention in scramble/emergency-base-defense (vimy-d9q) because when
// the CY / WF / refinery is actively taking damage, keeping the attack
// squad "reserved" for a future push is worse than losing the critical
// infrastructure. Game 59: 2 rocket launchers destroyed the CY while an
// idle mammoth tank sat in the ground-attack squad, never engaged
// because scramble excludes squad members and emergency needs zero idle.
func ActionDefendCriticalBuilding(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.nearestEnemyAttackingCritical()
	if enemy == nil {
		return nil
	}
	// Pull all near-base ground units regardless of squad membership.
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

// ActionEmergencyDefendBase redirects nearby ground units (regardless of idle
// status) to defend the base. Used when no idle units are available but units
// near the base have stale orders and aren't responding to the attack.
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

// repairToggleEntry tracks the last RepairBuilding toggle issued per building
// so we don't re-toggle (cancel) an in-flight repair every eval. The repair
// command in OpenRA is a toggle: send once to start, send again to STOP.
// Without per-building idempotency, repair-buildings firing thousands of
// times in a long match was constantly toggling repairs on and off, wasting
// cash and never actually completing.
type repairToggleEntry struct {
	Tick int
}

// repairResendTicks is how long we trust an in-flight repair toggle. If the
// building is still damaged this long after we issued, either the toggle was
// lost or repair stopped — re-issue.
const repairResendTicks = 1500

// criticalRepairTypes are buildings worth repair-spending even when we're
// under pressure (rush or sustained harassment). Production infrastructure
// and economy nodes only — non-critical buildings (radar, helipad, tech
// center, etc.) lose repair budget so cash flows to combat units instead.
// vimy-rw8 follow-up after game 41 (4900 repair firings vs 122 produce-
// infantry — repair budget was crushing production cash).
func isCriticalRepairType(t string) bool {
	switch baseTypeName(t) {
	case ConstructionYard, WarFactory, AlliedBarracks, SovietBarracks, Refinery, PowerPlant, AdvancedPower:
		return true
	}
	return false
}

// RepairBudgetRatio is exposed as an env memory value ("repairBudgetRatio",
// float64) so ActionRepairDamagedBuildings can honor the doctrine-level
// repair_budget_ratio knob. When set to a positive value <= 1.0, we only
// initiate a NEW repair toggle if remaining cash after repair (approximated
// as current cash) is above (1 - ratio) * currentCash — i.e. the doctrine
// wants to keep (1 - ratio) fraction of cash reserved for production.
// repairCashFloor: no new repairs or re-toggles if cash is below this
// threshold. Diagnostic on game 62 showed cash pinned at 0 for the entire
// mid-game because in-flight repairs were consuming income the instant it
// converted from ore. Above-floor repairs still get through; at-floor
// repairs stop entirely so cash can accumulate for production.
const repairCashFloor = 500

// repairMaxConcurrent caps how many buildings we simultaneously repair.
// OpenRA drains cash per-tick per active repair. Uncapped means all N
// damaged buildings drain simultaneously and the sum crushes income.
// Cap of 2 keeps drain bounded so income can still support production.
const repairMaxConcurrent = 2

// RepairBudgetRatio is exposed as an env memory value ("repairBudgetRatio",
// float64) so ActionRepairDamagedBuildings can honor the doctrine-level
// repair_budget_ratio knob. When set to a positive value <= 1.0, we only
// initiate a NEW repair toggle if remaining cash after repair (approximated
// as current cash) is above (1 - ratio) * currentCash — i.e. the doctrine
// wants to keep (1 - ratio) fraction of cash reserved for production.
func ActionRepairDamagedBuildings(env RuleEnv, conn *ipc.Connection) error {
	// Hard cash floor. If we can't afford the floor, don't spend a dollar
	// more on repairs — production and rebuild need what remains.
	if env.State.Player.Cash < repairCashFloor {
		return nil
	}

	state := memoryMap[int, repairToggleEntry](env.Memory, "repairToggleSent")
	underPressure := env.IsRushed() || env.IsHarvesterHarassed()
	budgetRatio, _ := env.Memory["repairBudgetRatio"].(float64)
	// Budget = fraction of cash the doctrine allows repair to touch. Zero
	// disables (unlimited repair, current behavior). Values above 0 gate
	// new repair starts on cash >= reserveThreshold.
	reserveOK := true
	if budgetRatio > 0 && budgetRatio < 1.0 {
		// Reserve (1 - budgetRatio) of cash for production. A rush setting
		// 0.2 keeps 80% of cash for units. Cheap approximation: allow
		// starting new repairs only when current cash comfortably exceeds
		// the reserve floor. Cash floor scales with number of damaged
		// buildings (rough proxy for repair cost).
		damagedCount := len(env.DamagedBuildings())
		perBuildingRepairAllowance := 100 // rough estimate per damaged building
		neededHeadroom := int(float64(damagedCount*perBuildingRepairAllowance) / budgetRatio)
		if env.State.Player.Cash < neededHeadroom {
			reserveOK = false
		}
	}

	// Count currently-tracked active repairs so we can enforce a concurrency
	// cap. Anything older than repairResendTicks is considered stale/complete.
	activeRepairs := 0
	for _, entry := range state {
		if env.State.Tick-entry.Tick < repairResendTicks {
			activeRepairs++
		}
	}

	for _, b := range env.DamagedBuildings() {
		// When under pressure, only spend cash on infrastructure that keeps
		// us in the game. Radar/tech/helipad damage is acceptable; lost
		// production is not.
		if underPressure && !isCriticalRepairType(b.Type) {
			continue
		}
		// Doctrine-level repair budget: skip new repairs when reserve isn't
		// met. Already-issued repair toggles (idempotency check below) still
		// let in-flight repairs complete.
		prev, sent := state[b.ID]
		if !reserveOK && !sent {
			continue
		}
		if sent && env.State.Tick-prev.Tick < repairResendTicks {
			continue
		}
		// Concurrency cap: only initiate a new repair if we're below the
		// cap. Existing tracked repairs (already counted above) will finish
		// on their own; we're only rate-limiting new starts.
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
// when no enemy is currently visible. Uses AttackMove so scouts engage anything
// they spot en route, not just walk past. Each scout gets its own persistent
// round-robin patrol assignment (same machinery as ActionScoutPatrol) so
// repeat calls don't spam fresh orders that cancel the in-flight path — the
// earlier stateless implementation produced 772 attack_move commands in one
// game and kept the cleanup force bouncing between corners without arriving.
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

// generateWaypoints creates a 9-point search pattern (center, corners, edges)
// with a small margin to avoid map-edge pathing issues. When a terrain grid is
// available, waypoints in Water or Cliff zones are filtered out so ground
// scouts only visit reachable positions.
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
		{midX, midY},   // center
		{minX, minY},   // top-left
		{maxX, minY},   // top-right
		{maxX, maxY},   // bottom-right
		{minX, maxY},   // bottom-left
		{midX, minY},   // top-mid
		{maxX, midY},   // right-mid
		{midX, maxY},   // bottom-mid
		{minX, midY},   // left-mid
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

// ActionScoutPatrol patrols the map perimeter with each idle scout — Allied
// Rangers, or the designated light tank / attack dog for other factions.
// Each scout sticks to its assigned waypoint until it arrives (within
// scoutArriveRadius), then rotates to the next one. Move commands are
// throttled so we don't re-issue the same destination every tick — without
// this, each tick cancelled the in-flight patrol path and scouts never
// traversed the map.
func ActionScoutPatrol(env RuleEnv, conn *ipc.Connection) error {
	waypoints := generateWaypoints(env.State.MapWidth, env.State.MapHeight, env.Terrain)
	if len(waypoints) == 0 {
		return nil
	}

	scouts := env.IdleScouts()
	state := getScoutMoveState(env.Memory)

	// scout_reach_priority doctrine knob: when > 0.5 AND we already have
	// enemy base intel, override the round-robin patrol and send scouts
	// directly to the enemy base (or on a direct probe toward its position).
	// A rush needs the enemy base found FAST — perimeter patrol wastes ticks
	// visiting corners we don't care about once we know where the enemy is.
	scoutReach, _ := env.Memory["scoutReachPriority"].(float64)
	targetKnownBase := scoutReach > 0.5 && env.NearestEnemyBase() != nil

	for _, s := range scouts {
		prev, assigned := state[s.ID]
		// Stall check: if we've been telling this scout to go to the same
		// waypoint and it hasn't moved, the path is unreachable (chokepoint,
		// terrain lock). Advance to the next waypoint rather than retrying
		// the same bad target forever — exactly what trapped a dog at (64,64)
		// trying to reach (5,5) for the whole game before this check existed.
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
		// scout_reach override: point at the enemy base instead of the
		// next patrol waypoint. Keeps the throttle/stall machinery working
		// because destination is checked against prev.X/prev.Y as normal.
		if targetKnownBase {
			base := env.NearestEnemyBase()
			wp = [2]int{base.X, base.Y}
		}

		// Throttle: if we issued the same destination recently, don't resend —
		// the path is still valid and a fresh Move would cancel it.
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

// nextPatrolIdx returns the next patrol waypoint index for an actor rotating
// round-robin through a waypoint list. Advances when the actor has arrived
// within arriveRadius of its previous assignment, or when the caller forces
// advance (stall detected, TTL expired). For the first assignment
// (`assigned == false`) the caller should instead take an index from the
// shared pool via takePatrolPoolIdx — this function returns prevIdx in that
// case which is meaningless, but the caller will overwrite it. Shared between
// dedicated scouts and APCs-in-scout-mode so both behave identically: systematic
// rotation visits every waypoint, no blind spots on mid-edges.
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

// takePatrolPoolIdx returns the next index from a shared fan-out pool and
// increments it, so multiple actors assigned at the same tick start at
// different waypoints instead of stacking on waypoint 0.
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

// scoutMoveResend — scouts need to keep moving, so the window is shorter than
// APC deliveries. A scout stuck mid-patrol gets a fresh Move faster; one that
// arrives and goes idle will rotate to the next waypoint (different
// destination → throttle inactive, sends immediately).
const scoutMoveResend = 40
const scoutArriveRadius = 6.0

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

// captureOrderEntry records the last Capture order we issued for an engineer.
// Same reason as sendAPCMove: re-sending an identical Capture order every tick
// cancels the current walk-and-capture activity in OpenRA and resets it, so
// the engineer never arrives at the target. Observed live: 111 captures of
// the same (engineer, target) pair in a single game — zero completed.
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
	// Pick the engineer closest to the target so recently-unloaded engineers
	// capture instead of being re-loaded into a different APC.
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

// harvestResend throttles repeated harvest orders to the same harvester.
// Without this, a harvester that goes idle for one tick (between unload and
// the next round trip, while waiting to dock, etc.) gets a fresh harvest
// order every evaluation — which cancels in-flight pathing and can stall
// the harvester next to the refinery indefinitely (vimy-oli).
const harvestResend = 120

type harvestEntry struct {
	Tick int
	X    int
	Y    int
}

func ActionSendIdleHarvesters(env RuleEnv, conn *ipc.Connection) error {
	// Collect refineries; harvesters are dispatched round-robin across them
	// so multiple idle harvesters spread out instead of all converging on
	// the first refinery's coordinates.
	var refineries []model.Building
	for i := range env.State.Buildings {
		if matchesType(env.State.Buildings[i].Type, Refinery) {
			refineries = append(refineries, env.State.Buildings[i])
		}
	}
	state := memoryMap[int, harvestEntry](env.Memory, "harvestSent")
	for i, u := range env.IdleHarvesters() {
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
// These use the Defense queue despite being buildings — matches OpenRA's
// categorization of superweapons as "defense" items.

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

// minelayerTarget records where a minelayer was last tasked to lay mines.
// Used by updateMinelayers to detect "mission complete" (minelayer is idle
// and near its target) and clear the assignment so the rule can re-task it
// to a different chokepoint. Without this, a minelayer that successfully
// laid its minefield stayed flagged `assigned` forever and never got
// another task.
type minelayerTarget struct {
	X, Y       int
	AssignedAt int
}

func getMinelayerTargets(memory map[string]any) map[int]minelayerTarget {
	return memoryMap[int, minelayerTarget](memory, "minelayerTargets")
}

// minelayerDoneRadius: distance in cells within which we consider a minelayer
// to have reached (and placed) its assigned minefield. Small — the unit lays
// mines in a 3x3 pattern centered on the target, so idle within 3 cells of
// target == mission complete.
const minelayerDoneRadius = 4

// ActionLayMines sends idle minelayers to lay mines. Priority order:
//  1. Chokepoints (bridges / narrow land strips) ranked toward the nearest
//     known enemy base — block the structural traffic funnel.
//  2. Fraction-of-the-way toward the enemy when we have intel but no chokes.
//  3. Compass perimeter around the base when we have neither.
// Each minelayer is tracked in memory so we don't re-issue orders every tick.
// The minelayer auto-rearms at the service depot when out of ammo (handled
// by OpenRA's LayMines activity).
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

	// Rotating choke cursor so successive re-tasks cycle through chokepoints
	// instead of re-laying on top of the previously mined one. Advances once
	// per minelayer dispatched.
	chokeIdx, _ := env.Memory["mineChokeIdx"].(int)

	for i, m := range miners {
		var tx, ty int

		switch {
		case len(chokes) > 0 && env.Terrain != nil:
			// Spread minelayers across the top-ranked chokes, rotating the
			// cursor across retasks so a re-deployed minelayer visits a
			// different choke than the one it already mined.
			c := chokes[(chokeIdx+i)%len(chokes)]
			tx, ty = env.Terrain.ZoneCenter(c.Col, c.Row)
		case base != nil:
			// Mine toward the enemy — block the most likely attack path.
			fraction := 0.25 + 0.05*float64(i)
			if fraction > 0.40 {
				fraction = 0.40
			}
			tx = centX + int(float64(base.X-centX)*fraction)
			ty = centY + int(float64(base.Y-centY)*fraction)
		default:
			// No intel — lay a defensive perimeter around the base.
			// Place mines at ~15% of map diagonal from base centroid,
			// cycling through compass directions for each minelayer.
			mw := float64(env.State.MapWidth)
			mh := float64(env.State.MapHeight)
			perimeterDist := math.Sqrt(mw*mw+mh*mh) * 0.15
			angle := float64(i) * (2 * math.Pi / 4) // N, E, S, W
			tx = centX + int(perimeterDist*math.Cos(angle))
			ty = centY + int(perimeterDist*math.Sin(angle))
		}

		// Clamp to map bounds.
		tx = max(1, min(tx, env.State.MapWidth-2))
		ty = max(1, min(ty, env.State.MapHeight-2))

		// Lay a small 3x3 rectangular minefield centered on target.
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

// updateMinelayers clears assignments for dead minelayers and for minelayers
// that have reached their minefield target (mission complete — they've
// placed the mines and are now idle). Without the "mission complete" clear,
// a minelayer that successfully laid its minefield stayed flagged `assigned`
// forever and could never be retasked, leaving it parked on its own minefield
// for the rest of the game. Called each tick from the engine.
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
			// No target recorded (e.g. reloaded state without coords) — leave
			// the assignment alone; ActionLayMines will record on the next
			// retask after the unit becomes idle via some other path.
			continue
		}
		// Mission complete: minelayer is idle within a small radius of the
		// target it was sent to. Clear both maps so IdleMinelayers returns
		// it and ActionLayMines can retask it to the next rotated choke.
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

// memoryMap fetches (creating if absent) a typed map stored under key in the
// rule engine's memory. Used for per-actor throttle/tracking tables — the
// "allocate on first touch" pattern that previously lived as a separate
// getXxxState function per order type.
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

// Unload tuning. 10 cells is roomy enough to tolerate APC pathing stalling
// a few cells short of a building; the engineer walks the rest on foot.
const unloadNearTargetCells = 10

// apcProgressEntry tracks an APC's position between ticks so we can detect
// a stall (APC not moving despite a pending move order) and unload anyway.
type apcProgressEntry struct {
	LastTick int
	LastX    int
	LastY    int
}

// apcExploreEntry remembers which waypoint an APC is heading for when no
// capturable is visible. Without this, every tick we'd pick a fresh waypoint
// and the APC would never arrive anywhere. Idx is the round-robin position in
// the patrol cycle; with systematic rotation every waypoint eventually gets
// visited regardless of where the APC starts. The old farthest-first heuristic
// oscillated between opposite corners and left mid-edges permanently blind.
type apcExploreEntry struct {
	X          int
	Y          int
	AssignedAt int
	Idx        int
}

// apcStallTicks: how long the APC must sit at the exact same tile, after we
// first observed it there, before we give up and unload in place. 50 was too
// aggressive — OpenRA can take several seconds to start pathing under load
// (pending Enter orders, blocked adjacent cells, queue processing), and a
// premature unload dumps the engineer at base where it has to walk across
// the map on foot and usually dies before reaching a capturable. 400 ticks
// ≈ 16 seconds at 25 Hz — long enough that a healthy APC will have moved,
// short enough that a genuinely stuck APC eventually recovers.
const apcStallTicks = 400
const apcExploreTTL = 600         // re-pick exploration target after this many ticks
const apcExploreArriveRadius = 12 // how close "arrived" counts for exploration

// apcScoutStallTicks: how long the APC must sit at the same tile while assigned
// to an exploration waypoint before we assume the target is unreachable and
// re-pick. Shorter than apcStallTicks because unloading-in-place makes sense
// when capturing (at least the engineer gets out) but is useless when scouting
// (the loaded engineer would walk alone into fog).
const apcScoutStallTicks = 200

// getAPCProgress returns the map (allocating if needed) tracking APC positions.
func getAPCProgress(memory map[string]any) map[int]apcProgressEntry {
	return memoryMap[int, apcProgressEntry](memory, "apcDeliveryProgress")
}

// getAPCExploreTargets returns the map of APC → exploration waypoint.
func getAPCExploreTargets(memory map[string]any) map[int]apcExploreEntry {
	return memoryMap[int, apcExploreEntry](memory, "apcExploreTarget")
}

// apcIsStalled reports whether an APC has been sitting at the same tile for
// at least apcStallTicks. Also updates the tracked position as a side effect.
func apcIsStalled(memory map[string]any, u model.Unit, tick int) bool {
	return apcStalledFor(memory, u, tick, apcStallTicks)
}

// apcScoutStalled reports whether a scouting APC has been sitting at the same
// tile long enough that we should assume the current exploration waypoint is
// unreachable and re-pick. Shares tracking with apcIsStalled — we only want
// one source of truth for "is this APC making progress."
func apcScoutStalled(memory map[string]any, u model.Unit, tick int) bool {
	return apcStalledFor(memory, u, tick, apcScoutStallTicks)
}

func apcStalledFor(memory map[string]any, u model.Unit, tick, threshold int) bool {
	return actorStalledAt(memory, "apcDeliveryProgress", u, tick, threshold)
}

// actorStalledAt reports whether an actor has held the same tile for at least
// `threshold` ticks, tracked under memKey. Shared by APC delivery/scout and
// ground-scout patrols — any action that issues a Move and needs to know
// whether the target is unreachable (chokepoint, terrain lock) so it can
// advance to a different waypoint instead of retrying forever.
func actorStalledAt(memory map[string]any, memKey string, u model.Unit, tick, threshold int) bool {
	m := memoryMap[int, apcProgressEntry](memory, memKey)
	entry, ok := m[u.ID]
	if !ok || entry.LastX != u.X || entry.LastY != u.Y {
		m[u.ID] = apcProgressEntry{LastTick: tick, LastX: u.X, LastY: u.Y}
		return false
	}
	return tick-entry.LastTick >= threshold
}

// scoutStallTicks mirrors apcScoutStallTicks — same tradeoff: long enough to
// tolerate normal path congestion, short enough to recover from a genuinely
// unreachable waypoint within ~8 seconds.
const scoutStallTicks = 200

// scoutStalled reports whether a ground scout has been sitting at the same
// tile long enough to assume its current patrol waypoint is unreachable.
// Uses a separate memory slot from APC tracking so the two don't collide.
func scoutStalled(memory map[string]any, u model.Unit, tick int) bool {
	return actorStalledAt(memory, "scoutProgress", u, tick, scoutStallTicks)
}

// clearAPCTracking wipes per-APC state after unload so a new cycle starts
// from scratch. Also clears the cargo intent tag — once the APC is empty,
// the next load action is free to re-tag it for a different mission.
func clearAPCTracking(memory map[string]any, id int) {
	delete(getAPCProgress(memory), id)
	delete(getAPCExploreTargets(memory), id)
	delete(getAPCMoveState(memory), id)
	ClearAPCCargoIntent(memory, id)
}

// apcMoveEntry records the last Move order we issued for an APC so we
// don't re-send it every tick. OpenRA treats each fresh Move as a new
// order that cancels the current activity — so repeated sends with the
// same destination reset pathfinding every tick and pin the APC in place.
type apcMoveEntry struct {
	Tick int
	X    int
	Y    int
}

// apcMoveResend — if the APC is still being asked to go to the same spot
// this long after our last send, assume the prior order was overridden
// (stuck, blocked, attacked) and resend.
const apcMoveResend = 60

func getAPCMoveState(memory map[string]any) map[int]apcMoveEntry {
	return memoryMap[int, apcMoveEntry](memory, "apcMoveSent")
}

// sendAPCMove issues a Move command for an APC, throttled so we don't
// re-send the same destination every tick (which cancels the in-flight
// path in OpenRA). Returns nil without sending if a recent identical
// order is still in flight.
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

	// No visible capturable — use the loaded APC as a scout, rotating
	// through map waypoints until a capturable comes into sight.
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

// advanceAPCPatrol picks the APC's next patrol waypoint, mirroring
// ActionScoutPatrol's round-robin behavior: systematic rotation through the
// waypoint list, advancing on arrival or on stall/TTL expiry. Prior
// farthest-first logic oscillated between opposite corners and never visited
// mid-edges, so enemy bases along map edges never got sighted.
func advanceAPCPatrol(env RuleEnv, apc model.Unit, prev apcExploreEntry, assigned bool, waypoints [][2]int) apcExploreEntry {
	forceAdvance := false
	if assigned {
		if env.State.Tick-prev.AssignedAt > apcExploreTTL {
			forceAdvance = true
		}
		if apcScoutStalled(env.Memory, apc, env.State.Tick) {
			forceAdvance = true
			// Reset stall tracking so the next waypoint gets a fair window.
			delete(getAPCProgress(env.Memory), apc.ID)
		}
	}
	idx := nextPatrolIdx(apc.X, apc.Y, prev.X, prev.Y, prev.Idx, len(waypoints), apcExploreArriveRadius, assigned, forceAdvance)
	if !assigned {
		idx = takePatrolPoolIdx(env.Memory, "apcPatrolIdx", len(waypoints))
	}
	// Preserve AssignedAt when the index didn't change so the TTL check
	// measures dwell time on the current waypoint, not time since the action
	// last ran.
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

// ActionLoadCombatInfantry loads one idle combat infantry into an idle empty
// APC each tick. Skips squad-assigned infantry so we don't steal from attack squads.
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
		// Engineer tag is sticky: both load actions target the first idle empty
		// APC on the same tick, and load-engineer-into-apc (845) fires before
		// load-assault-infantry (838). If an engineer already claimed this APC
		// this tick, don't overwrite — the APC is a capture mission, even if
		// combat infantry also piles in.
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

// ActionDeliverAssaultAPC moves loaded APCs toward the nearest known enemy
// base. Unloads when within 7 cells, otherwise moves closer. Skips water-based
// intel (e.g. naval yard) since APCs are ground units.
//
// When no land target is known (intel-less game, scouts only found enemy
// units not buildings), the APC falls back to exploration — same scout-with-
// loaded-APC pattern as the engineer path. This keeps combat-loaded APCs from
// sitting at base forever waiting for a building sighting that never comes.
func ActionDeliverAssaultAPC(env RuleEnv, conn *ipc.Connection) error {
	apcs := env.IdleCombatLoadedAPCs()
	if len(apcs) == 0 {
		return nil
	}

	// Find a valid land target — skip water-based intel (e.g. naval yard).
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

	// No land target — scout with the loaded APC. Once it discovers an enemy
	// building or unit, the branch above takes over on the next tick.
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
// These cap the number of units sent per order, keeping some as reserves.
// Superseded by squad-based actions in compiled doctrines but still used
// by the seed rule set.

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

// --- Squad action factories ---

// FormSquad assigns unit IDs to a named squad in memory but does NOT issue
// orders. Formation and action are separate rules so the compiler can set
// different priorities and conditions for each (e.g. form at priority+5,
// act at priority).
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
			slog.Info("squad reinforced", "name", name, "added", add, "size", len(sq.UnitIDs), "target", sq.TargetSize)
			return nil
		}

		// Initial formation: require full size.
		if len(pool) < size {
			return nil
		}
		ids := make([]int, size)
		for i := range size {
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
		slog.Info("squad formed", "name", name, "domain", domain, "role", role, "size", size)
		return nil
	}
}

// huntBaseState tracks which radial position a squad is cycling through
// when hunting around an enemy base. Stored in memory per squad name.
type huntBaseState struct {
	BaseX, BaseY int
	Step         int
}

// huntOffset converts a hunt step into an (dx, dy) offset from the base centroid.
// Step 0 returns (0,0) — the centroid itself. Steps 1-16 produce two concentric
// rings of 8 positions each, spaced 45° apart:
//
//	Steps 1-8:   inner ring at radius R   (ring = 1)
//	Steps 9-16:  outer ring at radius 2R  (ring = 2)
//
// idx selects one of 8 compass points (0-7) via modular arithmetic.
// ring selects which concentric circle via integer division.
// The squad sweeps close to the centroid first (catching buildings just
// inside fog of war), then widens to find outlying structures.
func huntOffset(step, radius int) (int, int) {
	if step <= 0 {
		return 0, 0
	}
	idx := (step - 1) % 8        // which of 8 compass points (0-7)
	ring := (step-1)/8 + 1       // which ring: 1 for steps 1-8, 2 for 9-16
	r := float64(radius * ring)  // inner ring = R, outer ring = 2R
	angle := float64(idx) * 2 * math.Pi / 8
	return int(r * math.Cos(angle)), int(r * math.Sin(angle))
}

// squadAttackState remembers whether a squad has already committed to a
// specific target, so the rally-then-attack behavior only requires clumping
// on the initial deploy, not on every eval mid-attack. Without this the
// squad would oscillate: attack -> spread -> regroup -> attack -> spread.
type squadAttackState struct {
	TargetX, TargetY int
	Attacking        bool // false = still regrouping to centroid
	LastTick         int
}

const (
	// squadAttackCommitTTL: how long a target commitment stays "sticky"
	// before we re-evaluate clumping. Long enough to complete a typical
	// engagement, short enough to re-regroup if the squad's next target
	// is different and requires a fresh assembly.
	squadAttackCommitTTL = 2000
	// squadRallyRadius: 80% of squad members must be within this many map
	// cells of the centroid before the squad is considered "clumped."
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

		// Rally-then-attack. On a fresh commit (target changed or stale
		// state), if the squad is not clumped, issue AttackMove to the squad
		// centroid so trailing units catch up and leading units halt/fall
		// back. Only when 80% of members are within squadRallyRadius do we
		// commit to the actual attack. Once committed, we don't re-regroup
		// for squadAttackCommitTTL ticks — the squad fights as long as the
		// target holds and we don't want mid-fight oscillation.
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

		// vimy-zyv: when there's a hot defense corridor between the squad and
		// the target, route via the threat-aware waypoint instead of attack-
		// moving straight through the cluster. ApproachWaypoint already gates
		// itself on cumulative threat in the bounding box, so when the path
		// is clear it returns false and we fall through to direct routing.
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

// attackOrderEntry tracks the last TypeAttack order issued for an actor so we
// don't re-send identical attack orders every tick. OpenRA treats each new
// attack order as cancel-and-restart, so constant re-issuance pins units in
// place unable to close and fire. Same failure mode we fixed for Move and
// Capture; adding it here for squad combat actions.
type attackOrderEntry struct {
	Tick     int
	TargetID int
}

const attackOrderResend = 60

// sendAttack issues a TypeAttack order for actorID targeting targetID,
// throttled so identical re-issuances within attackOrderResend ticks are
// suppressed.
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

// sendAttackMove batches a TypeAttackMove to (x,y) across actorIDs, skipping
// any actor that already has an identical in-flight order within
// attackMoveResend ticks. Without this, a squad action re-issues the same
// AttackMove every tick and each command cancels the in-flight path — units
// stall mid-map and never reach their target.
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

		// Rally-then-attack (long-walk case). Base-attack is the biggest
		// dispersal risk: squads walk far from home, fast units (tanks,
		// dogs) arrive first and get picked off before rifles catch up.
		// Same rally state as SquadAttackMove, keyed by the base position.
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

		// Retrieve or initialize hunt state for this squad.
		memKey := "huntBase:" + name
		state, _ := env.Memory[memKey].(*huntBaseState)
		if state == nil {
			state = &huntBaseState{}
		}

		// Reset to step 0 (approach centroid) when base intel changes.
		if state.BaseX != base.X || state.BaseY != base.Y {
			state.BaseX = base.X
			state.BaseY = base.Y
			state.Step = 0
		}

		tx, ty := base.X, base.Y

		// On the initial approach (step 0), check for a threat-aware waypoint
		// — a safer entry zone skirting remembered enemy defenses. If the
		// squad isn't already near the waypoint, route through it instead of
		// straight at the base. Once they arrive, subsequent ticks hit the
		// base centroid as normal.
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

		if state.Step > 0 {
			// Compute aggression-scaled radius.
			mapDim := max(env.State.MapWidth, env.State.MapHeight)
			baseRadius := mapDim / 16
			scale := 0.25 + aggression*1.25
			radius := int(float64(baseRadius) * scale)
			if radius < 1 {
				radius = 1
			}

			dx, dy := huntOffset(state.Step, radius)
			tx = base.X + dx
			ty = base.Y + dy

			// Clamp to map bounds.
			tx = max(0, min(tx, env.State.MapWidth-1))
			ty = max(0, min(ty, env.State.MapHeight-1))

			// Terrain check for ground/naval squads — skip water/cliff.
			squads := getSquads(env.Memory)
			sq := squads[name]
			if sq != nil && sq.Domain != "air" && env.Terrain != nil {
				t := env.Terrain.AtMapPos(tx, ty)
				if t != model.Land && t != model.Bridge {
					tx, ty = base.X, base.Y // fallback to centroid
				}
			}
		}

		// Throttle: if the squad already has an identical in-flight order to
		// (tx,ty) within attackMoveResend ticks, suppress the send AND the
		// step advance — otherwise the 16-step hunt rotates once per tick and
		// units never reach any step's destination.
		if !attackMoveHasFreshTarget(env, ids, tx, ty) {
			env.Memory[memKey] = state
			return nil
		}

		slog.Debug("squad attacking known base", "squad", name, "count", len(ids),
			"owner", base.Owner, "step", state.Step, "x", tx, "y", ty)

		// Advance step: 0→1, 1→2, ..., 16→1 (wrap, skip 0 on subsequent cycles).
		if state.Step >= 16 {
			state.Step = 1
		} else {
			state.Step++
		}
		env.Memory[memKey] = state

		return sendAttackMove(env, conn, ids, tx, ty)
	}
}

// attackMoveHasFreshTarget reports whether any actor in ids would actually
// receive a new AttackMove if sent to (x,y) — i.e. at least one actor's
// throttle window has expired. Used by squad actions that also need to
// advance internal state only when a send will land, not on every tick.
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

// squadCentroid returns the average position of all living squad members,
// and false if the squad is empty or unknown.
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
	var ids []uint32
	for _, id := range sq.UnitIDs {
		_, isRetreating := retreating[id]
		if idleSet[id] && !isRetreating {
			ids = append(ids, uint32(id))
		}
	}
	return ids
}

// --- Micro action factories ---

// RetreatDamagedUnits sends Move (not AttackMove) for each damaged combat unit.
// Routes vehicles to the service depot (auto-repair); infantry and others to
// the base centroid (safety behind defenses). Marks retreating units in memory
// so focus-fire and squad-attack rules skip them.
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
				// Send aircraft to airfield/helipad for repair.
				slog.Debug("retreating damaged aircraft to airfield", "id", u.ID, "type", u.Type,
					"hp_ratio", float64(u.HP)/float64(u.MaxHP), "airfield", airfield.ID)
				if err := conn.Send(ipc.TypeRepairUnit, ipc.RepairUnitCommand{
					ActorID:          uint32(u.ID),
					RepairBuildingID: uint32(airfield.ID),
				}); err != nil {
					return err
				}
			} else if depot != nil && !isAircraft(u) && !isNaval(u) {
				// Send repair order — unit will enter the depot pad and heal.
				slog.Debug("retreating damaged unit to depot", "id", u.ID, "type", u.Type,
					"hp_ratio", float64(u.HP)/float64(u.MaxHP), "depot", depot.ID)
				if err := conn.Send(ipc.TypeRepairUnit, ipc.RepairUnitCommand{
					ActorID:          uint32(u.ID),
					RepairBuildingID: uint32(depot.ID),
				}); err != nil {
					return err
				}
			} else {
				// Fallback: move to centroid (naval or no repair building).
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

// ClearHealedUnits removes healed, dead, or timed-out units from the
// retreating set, returning them to the combat pool. The timeout prevents
// permanent unit leaks when repair fails (e.g. depot destroyed mid-repair).
func ClearHealedUnits(hpThreshold float64) ActionFunc {
	const retreatTimeout = 200 // ticks before forcing release

	return func(env RuleEnv, conn *ipc.Connection) error {
		retreating := getRetreatingUnits(env.Memory)
		if len(retreating) == 0 {
			return nil
		}
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
		ids := squadIdleActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}
		centX, centY := env.BuildingCentroid()
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

// SquadFocusFire sends individual Attack commands for each idle squad member
// targeting the best ground target. Concentrates damage on the highest-value
// enemy for faster kills.
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

// SquadAirStrike sends individual Attack commands for each idle air squad member
// targeting the best air target (defense structures, production, etc.).
// Concentrates all aircraft on the highest-value enemy for maximum impact.
//
// vimy-zyv: when there's a heavy AA corridor between the squad and target,
// route the squad to a low-AA staging point first via attack-move, only
// committing to direct Attack once the squad has reached the safer flank.
// Without this, aircraft fly straight through SAM clusters every time.
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

// airApproachWaypointFor returns an AA-aware staging point if the squad is
// far from `dest` AND a low-AA waypoint exists. Once the squad's centroid
// reaches the waypoint zone, returns false so callers fall through to
// direct attack on the target.
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

// FleeHarvesters sends Move toward the nearest refinery for each harvester
// in danger. Checks all harvesters (idle or not) — better to lose ore than
// the harvester.
// harvesterFleeEntry records the last flee order we issued for a harvester.
// Prevents re-issuing the same Move command every tick — each re-issue
// invalidates the server's pathfinder and effectively pins the harvester
// in place. Fresh orders only go out when the destination changes or the
// prior order is stale enough that we suspect it was overridden.
type harvesterFleeEntry struct {
	Tick int
	X    int
	Y    int
}

// harvesterFleeResend controls how long we trust an in-flight flee order.
// If a harvester is still in danger this long after our last flee, either
// the prior order was cancelled or the harvester is stuck — resend.
const harvesterFleeResend = 100

func getHarvesterFleeState(memory map[string]any) map[int]harvesterFleeEntry {
	return memoryMap[int, harvesterFleeEntry](memory, "harvesterFleeing")
}

// CountFleeingHarvesters returns how many harvesters are currently in an
// active flee cycle. Exposed so the event detector (agent package) can signal
// sustained harvester harassment without having to know the internal struct
// layout of the flee tracking map. Uses reflection so tests can populate the
// map with any int-keyed value type without taking a dependency on the
// unexported entry struct.
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
		if len(harvesters) == 0 {
			return nil
		}
		// Collect refinery positions for nearest-refinery lookup.
		var refineries []model.Building
		for _, b := range env.State.Buildings {
			if matchesType(b.Type, Refinery) {
				refineries = append(refineries, b)
			}
		}
		// Fallback to building centroid if no refineries.
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

		state := getHarvesterFleeState(env.Memory)
		// Drop entries for dead or no-longer-endangered harvesters so the
		// map doesn't grow unbounded.
		inDanger := make(map[int]bool, len(harvesters))
		for _, u := range harvesters {
			inDanger[u.ID] = true
		}
		for id := range state {
			if !inDanger[id] {
				delete(state, id)
			}
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
			// Skip if we already told this harvester to flee to the same
			// spot recently. Re-sending would cancel the in-flight pathing
			// and pin the harvester in place.
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

// unblockEgressResend throttles repeated nudges of the same war-factory
// blocker. Long enough for the move to complete on a normal OpenRA path but
// short enough that we retry promptly if the blocker is pinned (fighting,
// repairing) and can't leave.
const unblockEgressResend = 120

type egressEntry struct{ Tick int }

// ActionUnblockWarFactoryEgress scatters a friendly unit camped on the
// war-factory exit so a ready vehicle can emerge. Observed live (vimy-zh8):
// a completed tank stayed stuck behind a parked unit and every downstream
// produce-* rule hung on QueueBusy("Vehicle") for the rest of the game.
// Fires only when the Vehicle queue has an item at 100% and a war factory
// exists. The nudge target is the nearest non-harvester, non-MCV ground
// unit within 3 cells of the war factory; it's pushed ~6 cells along the
// factory-to-centroid axis so it moves toward the base interior rather
// than back through the blocked tile.
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
