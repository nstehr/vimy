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

func ActionProduceMCV(env RuleEnv, conn *ipc.Connection) error {
	slog.Debug("producing MCV — construction yard lost")
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  MCV,
		Count: 1,
	})
}

func ActionDeployMCV(env RuleEnv, conn *ipc.Connection) error {
	// Cooldown: the C# side needs time to process the deploy order.
	// Without this, the sidecar would spam deploy commands every tick.
	if lastTick, ok := env.Memory["deployMCVTick"].(int); ok {
		if env.State.Tick-lastTick < 50 {
			return nil
		}
	}
	for _, u := range env.State.Units {
		if matchesType(u.Type, MCV) && u.Idle {
			slog.Debug("deploying MCV", "id", u.ID)
			env.Memory["deployMCVTick"] = env.State.Tick
			return conn.Send(ipc.TypeDeploy, ipc.DeployCommand{
				ActorID: uint32(u.ID),
			})
		}
	}
	return nil
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

	// Generate 16 candidates in the perimeter annulus (100%-150% of radius).
	type candidate struct {
		x, y  int
		score float64
	}
	var candidates []candidate
	for i := range 16 {
		angle := float64(i) * 2 * math.Pi / 16
		r := radius * (1.0 + rand.Float64()*0.5)
		x := cx + int(r*math.Cos(angle))
		y := cy + int(r*math.Sin(angle))

		// Terrain filter.
		if env.Terrain != nil {
			t := env.Terrain.AtMapPos(x, y)
			if t != model.Land && t != model.Bridge {
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

		// Score: perimeter bonus (weight 0.25).
		distFromCenter := math.Sqrt(float64((x-cx)*(x-cx) + (y-cy)*(y-cy)))
		perimeterScore := distFromCenter / radius
		if perimeterScore > 1 {
			perimeterScore = 1
		}

		score := 0.35*threatScore + 0.15*protectionScore + 0.25*spreadScore + 0.25*perimeterScore
		candidates = append(candidates, candidate{x, y, score})
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
	for _, role := range []string{"cruiser", "destroyer"} {
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
	idle := env.IdleGroundUnits()
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

func ActionRepairDamagedBuildings(env RuleEnv, conn *ipc.Connection) error {
	for _, b := range env.DamagedBuildings() {
		slog.Debug("repairing building", "id", b.ID, "type", b.Type)
		if err := conn.Send(ipc.TypeRepairBuilding, ipc.RepairBuildingCommand{
			ActorID: uint32(b.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func ActionScoutWithIdleUnits(env RuleEnv, conn *ipc.Connection) error {
	waypoints := generateWaypoints(env.State.MapWidth, env.State.MapHeight, env.Terrain)
	if len(waypoints) == 0 {
		return nil
	}

	idx, _ := env.Memory["scoutWaypointIdx"].(int)
	wp := waypoints[idx%len(waypoints)]

	idle := env.IdleGroundUnits()
	n := min(2, len(idle))
	ids := make([]uint32, n)
	for i := range n {
		ids[i] = uint32(idle[i].ID)
	}

	slog.Debug("scouting with idle units", "count", n, "waypoint", wp, "wpIdx", idx%len(waypoints))

	env.Memory["scoutWaypointIdx"] = (idx + 1) % len(waypoints)

	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        wp[0],
		Y:        wp[1],
	})
}

// generateWaypoints creates a 9-point search pattern (center, corners, edges)
// with 10% margins to avoid map-edge pathing issues. When a terrain grid is
// available, waypoints in Water or Cliff zones are filtered out so ground
// scouts only visit reachable positions.
func generateWaypoints(mapW, mapH int, terrain *model.TerrainGrid) [][2]int {
	if mapW == 0 || mapH == 0 {
		return nil
	}
	marginX := mapW / 10
	marginY := mapH / 10
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

func ActionScoutWithRangers(env RuleEnv, conn *ipc.Connection) error {
	waypoints := generateWaypoints(env.State.MapWidth, env.State.MapHeight, env.Terrain)
	if len(waypoints) == 0 {
		return nil
	}

	idx, _ := env.Memory["rangerScoutIdx"].(int)
	rangers := env.IdleRangers()

	for _, r := range rangers {
		wp := waypoints[idx%len(waypoints)]
		slog.Debug("ranger scouting", "id", r.ID, "waypoint", wp, "wpIdx", idx%len(waypoints))
		if err := conn.Send(ipc.TypeMove, ipc.MoveCommand{
			ActorID: uint32(r.ID),
			X:       wp[0],
			Y:       wp[1],
		}); err != nil {
			return err
		}
		idx++
	}

	env.Memory["rangerScoutIdx"] = idx % len(waypoints)
	return nil
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

func ActionCaptureBuilding(env RuleEnv, conn *ipc.Connection) error {
	target := env.NearestCapturable()
	if target == nil {
		return nil
	}
	engineers := env.IdleEngineers()
	if len(engineers) == 0 {
		return nil
	}
	eng := engineers[0]
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

func ActionSendIdleHarvesters(env RuleEnv, conn *ipc.Connection) error {
	// Send toward refinery so the harvest command picks nearby ore patches.
	tx, ty := 0, 0
	for _, b := range env.State.Buildings {
		if matchesType(b.Type, Refinery) {
			tx, ty = b.X, b.Y
			break
		}
	}
	if tx == 0 && ty == 0 && len(env.State.Buildings) > 0 {
		tx = env.State.Buildings[0].X
		ty = env.State.Buildings[0].Y
	}
	for _, u := range env.IdleHarvesters() {
		slog.Debug("sending idle harvester", "id", u.ID)
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
	if base := env.NearestEnemyBase(); base != nil {
		x, y = base.X, base.Y
	} else if enemy := env.NearestEnemy(); enemy != nil {
		x, y = enemy.X, enemy.Y
	} else {
		return nil
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
	if base := env.NearestEnemyBase(); base != nil {
		x, y = base.X, base.Y
	} else if enemy := env.NearestEnemy(); enemy != nil {
		x, y = enemy.X, enemy.Y
	} else {
		return nil
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
		return nil
	}
	slog.Debug("producing APC", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueVehicle,
		Item:  item,
		Count: 1,
	})
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
	slog.Debug("loading engineer into APC", "engineer", engineers[0].ID, "apc", apcs[0].ID)
	return conn.Send(ipc.TypeEnterTransport, ipc.EnterTransportCommand{
		ActorID:     uint32(engineers[0].ID),
		TransportID: uint32(apcs[0].ID),
	})
}

func ActionUnloadAPCNearTarget(env RuleEnv, conn *ipc.Connection) error {
	target := env.NearestCapturable()
	if target == nil {
		return nil
	}
	apcs := env.IdleLoadedAPCs()
	if len(apcs) == 0 {
		return nil
	}
	// Find the APC closest to the capture target.
	best := apcs[0]
	bestDist := math.MaxFloat64
	for _, a := range apcs {
		dx := float64(a.X - target.X)
		dy := float64(a.Y - target.Y)
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			best = a
		}
	}
	dist := math.Sqrt(bestDist)
	if dist < 5 {
		slog.Debug("unloading APC near target", "apc", best.ID, "target", target.ID)
		return conn.Send(ipc.TypeUnload, ipc.UnloadCommand{ActorID: uint32(best.ID)})
	}
	// APC not close enough — move it toward the target.
	slog.Debug("moving APC toward target", "apc", best.ID, "target", target.ID, "dist", dist)
	return conn.Send(ipc.TypeMove, ipc.MoveCommand{
		ActorID: uint32(best.ID),
		X:       target.X,
		Y:       target.Y,
	})
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

		slog.Debug("squad attack-move", "squad", name, "count", len(ids), "target", enemy.ID)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids, X: enemy.X, Y: enemy.Y,
		})
	}
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

		slog.Debug("squad attacking known base", "squad", name, "count", len(ids),
			"owner", base.Owner, "step", state.Step, "x", tx, "y", ty)

		// Advance step: 0→1, 1→2, ..., 16→1 (wrap, skip 0 on subsequent cycles).
		if state.Step >= 16 {
			state.Step = 1
		} else {
			state.Step++
		}
		env.Memory[memKey] = state

		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids, X: tx, Y: ty,
		})
	}
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
		if idleSet[id] && !retreating[id] {
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
			retreating = make(map[int]bool)
		}

		depotX, depotY, hasDepot := env.ServiceDepotPos()
		centX, centY := env.BuildingCentroid()

		for _, u := range units {
			if isInfantry(u) {
				continue // infantry can't heal — no benefit to retreating
			}
			tx, ty := centX, centY
			if hasDepot && !isAircraft(u) && !isNaval(u) {
				tx, ty = depotX, depotY
			}
			slog.Debug("retreating damaged unit", "id", u.ID, "type", u.Type,
				"hp_ratio", float64(u.HP)/float64(u.MaxHP), "dest_x", tx, "dest_y", ty)
			if err := conn.Send(ipc.TypeMove, ipc.MoveCommand{
				ActorID: uint32(u.ID), X: tx, Y: ty,
			}); err != nil {
				return err
			}
			retreating[u.ID] = true
		}
		env.Memory["retreatingUnits"] = retreating
		return nil
	}
}

// ClearHealedUnits removes healed or dead units from the retreating set,
// returning them to the combat pool.
func ClearHealedUnits(hpThreshold float64) ActionFunc {
	return func(env RuleEnv, conn *ipc.Connection) error {
		retreating := getRetreatingUnits(env.Memory)
		if len(retreating) == 0 {
			return nil
		}
		aliveIDs := make(map[int]bool)
		for _, u := range env.State.Units {
			aliveIDs[u.ID] = true
			if retreating[u.ID] && u.MaxHP > 0 && float64(u.HP)/float64(u.MaxHP) >= hpThreshold {
				delete(retreating, u.ID)
				slog.Debug("unit healed, returning to duty", "id", u.ID, "type", u.Type)
			}
		}
		for id := range retreating {
			if !aliveIDs[id] {
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
			if err := conn.Send(ipc.TypeAttack, ipc.AttackCommand{
				ActorID:  id,
				TargetID: uint32(target.ID),
			}); err != nil {
				return err
			}
		}
		return nil
	}
}

// SquadAirStrike sends individual Attack commands for each idle air squad member
// targeting the best air target (defense structures, production, etc.).
// Concentrates all aircraft on the highest-value enemy for maximum impact.
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
		for _, id := range ids {
			slog.Debug("squad air strike", "squad", name, "unit", id, "target", target.ID)
			if err := conn.Send(ipc.TypeAttack, ipc.AttackCommand{
				ActorID:  id,
				TargetID: uint32(target.ID),
			}); err != nil {
				return err
			}
		}
		return nil
	}
}

// FleeHarvesters sends Move toward the nearest refinery for each harvester
// in danger. Checks all harvesters (idle or not) — better to lose ore than
// the harvester.
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
