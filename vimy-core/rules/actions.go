package rules

import (
	"log/slog"
	"strings"

	"github.com/nstehr/vimy/vimy-core/ipc"
)

func ActionDeployMCV(env RuleEnv, conn *ipc.Connection) error {
	for _, u := range env.State.Units {
		if strings.EqualFold(u.Type, MCV) {
			slog.Info("deploying MCV", "id", u.ID)
			return conn.Send(ipc.TypeDeploy, ipc.DeployCommand{
				ActorID: uint32(u.ID),
			})
		}
	}
	return nil
}

func ActionProducePowerPlant(env RuleEnv, conn *ipc.Connection) error {
	slog.Info("producing power plant")
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  PowerPlant,
		Count: 1,
	})
}

func ActionProduceRefinery(env RuleEnv, conn *ipc.Connection) error {
	slog.Info("producing refinery")
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  Refinery,
		Count: 1,
	})
}

func ActionProduceBarracks(env RuleEnv, conn *ipc.Connection) error {
	item := env.BuildableType("barracks")
	if item == "" {
		return nil
	}
	slog.Info("producing barracks", "item", item)
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceWarFactory(env RuleEnv, conn *ipc.Connection) error {
	slog.Info("producing war factory")
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueBuilding,
		Item:  WarFactory,
		Count: 1,
	})
}

func ActionPlaceBuilding(env RuleEnv, conn *ipc.Connection) error {
	for _, pq := range env.State.ProductionQueues {
		if strings.EqualFold(pq.Type, QueueBuilding) && pq.CurrentItem != "" && pq.CurrentProgress >= 100 {
			slog.Info("placing building", "item", pq.CurrentItem)
			return conn.Send(ipc.TypePlaceBuilding, ipc.PlaceBuildingCommand{
				Queue: QueueBuilding,
				Item:  pq.CurrentItem,
			})
		}
	}
	return nil
}

func ActionProduceInfantry(env RuleEnv, conn *ipc.Connection) error {
	slog.Info("producing infantry")
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueInfantry,
		Item:  RifleInfantry,
		Count: 1,
	})
}

func ActionAttackMoveIdleUnits(env RuleEnv, conn *ipc.Connection) error {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return nil
	}
	idle := env.IdleMilitaryUnits()
	ids := make([]uint32, len(idle))
	for i, u := range idle {
		ids[i] = uint32(u.ID)
	}
	slog.Info("attack-moving idle units", "count", len(ids), "target", enemy.ID)
	return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
		ActorIDs: ids,
		X:        enemy.X,
		Y:        enemy.Y,
	})
}

func ActionRepairDamagedBuildings(env RuleEnv, conn *ipc.Connection) error {
	for _, b := range env.DamagedBuildings() {
		slog.Info("repairing building", "id", b.ID, "type", b.Type)
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

	idle := env.IdleMilitaryUnits()
	n := min(2, len(idle))
	ids := make([]uint32, n)
	for i := range n {
		ids[i] = uint32(idle[i].ID)
	}

	slog.Info("scouting with idle units", "count", n, "waypoint", wp, "wpIdx", idx%len(waypoints))

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

func ActionSendIdleHarvesters(env RuleEnv, conn *ipc.Connection) error {
	// Heuristic: send harvesters toward the first refinery location, or base center.
	tx, ty := 0, 0
	for _, b := range env.State.Buildings {
		if strings.EqualFold(b.Type, Refinery) {
			tx, ty = b.X, b.Y
			break
		}
	}
	if tx == 0 && ty == 0 && len(env.State.Buildings) > 0 {
		tx = env.State.Buildings[0].X
		ty = env.State.Buildings[0].Y
	}
	for _, u := range env.IdleHarvesters() {
		slog.Info("sending idle harvester", "id", u.ID)
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
