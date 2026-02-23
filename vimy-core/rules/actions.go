package rules

import (
	"log/slog"
	"math"
	"math/rand"
	"strings"

	"github.com/nstehr/vimy/vimy-core/ipc"
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
	// Cooldown: don't retry deploy for 50 ticks (~20s) after sending.
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
			hx, hy := randomBaseOffset(env)
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

// randomBaseOffset picks a random point within a radius of the base centroid.
// The C# side uses this as the search center for placement validation.
func randomBaseOffset(env RuleEnv) (int, int) {
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

	// Pick a random point within the base radius (uniform disk sampling).
	angle := rand.Float64() * 2 * math.Pi
	r := radius * math.Sqrt(rand.Float64())
	return cx + int(r*math.Cos(angle)), cy + int(r*math.Sin(angle))
}

func ActionProduceDefense(env RuleEnv, conn *ipc.Connection) error {
	for _, role := range []string{"pillbox", "turret", "flame_tower", "tesla_coil"} {
		item := env.BuildableType(role)
		if item != "" {
			slog.Debug("producing defense", "item", item)
			return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
				Queue: QueueDefense,
				Item:  item,
				Count: 1,
			})
		}
	}
	return nil
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
	waypoints := generateWaypoints(env.State.MapWidth, env.State.MapHeight)
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

// generateWaypoints returns 9 map waypoints for scouting: center, 4 corners, 4 edge midpoints.
// Returns nil if map dimensions are zero.
func generateWaypoints(mapW, mapH int) [][2]int {
	if mapW == 0 || mapH == 0 {
		return nil
	}
	marginX := mapW / 10
	marginY := mapH / 10
	minX, maxX := marginX, mapW-marginX
	minY, maxY := marginY, mapH-marginY
	midX := mapW / 2
	midY := mapH / 2

	return [][2]int{
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
	// Heuristic: send harvesters toward the first refinery location, or base center.
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
