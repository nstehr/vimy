package rules

import (
	"log/slog"
	"math"
	"strings"

	"github.com/nstehr/vimy/vimy-core/ipc"
)

func ActionProduceMCV(env RuleEnv, conn CommandSender) error {
	slog.Debug("producing MCV — construction yard lost")
	return sendProduce(env, conn, QueueVehicle, MCV)
}

func ActionProducePowerPlant(env RuleEnv, conn CommandSender) error {
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

func ActionProduceRefinery(env RuleEnv, conn CommandSender) error {
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

func ActionProduceBarracks(env RuleEnv, conn CommandSender) error {
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

func ActionProduceWarFactory(env RuleEnv, conn CommandSender) error {
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

func ActionProduceRadar(env RuleEnv, conn CommandSender) error {
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

func ActionProduceAirfield(env RuleEnv, conn CommandSender) error {
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

func ActionProduceServiceDepot(env RuleEnv, conn CommandSender) error {
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

func ActionProduceNavalYard(env RuleEnv, conn CommandSender) error {
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
func ActionCancelStuckAircraft(env RuleEnv, conn CommandSender) error {
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

func ActionPlaceBuilding(env RuleEnv, conn CommandSender) error {
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

func ActionProduceInfantry(env RuleEnv, conn CommandSender) error {
	slog.Debug("producing infantry")
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: QueueInfantry,
		Item:  RifleInfantry,
		Count: 1,
	})
}

// Production resend intervals, chosen by whether the queue has work.
//
// `not queue-busy` used to throttle these by accident: one order made the queue
// busy and the rule stopped asking. A depth gate removed that, and game 112
// sent 1167 vehicle orders to field about 48 vehicles — most rejected for cost
// or a full queue, each rejection leaving the rule free to ask again.
//
// A flat hundred-tick throttle then overcorrected: game 113 ran the same
// doctrine and the same economy, sent 27 orders, and fielded two tanks against
// six. Because most orders are rejected, throttling attempts throttles
// successes in the same proportion — the spam was doing real work.
//
// So the interval depends on what the queue is doing. Nothing queued means the
// last order did not land, or the line has gone idle, and either way asking
// again shortly is right. Work in progress means a top-up can wait.
const produceRetryTicks = 15

const produceResendTicks = 100

// sendProduce issues a production order, spaced by the intervals above.
// Returning nil without sending is deliberate: the engine counts a rule as
// having acted only when something was sent, so a throttled rule reports what
// it did rather than inflating its own act count.
func sendProduce(env RuleEnv, conn CommandSender, queue, item string) error {
	sent := memoryMap[string, int](env.Memory, "produceSentTick")
	wait := produceResendTicks
	if env.QueueDepth(queue) == 0 {
		wait = produceRetryTicks
	}
	if last, ok := sent[queue]; ok && env.State.Tick-last < wait {
		return nil
	}
	sent[queue] = env.State.Tick
	return conn.Send(ipc.TypeProduce, ipc.ProduceCommand{
		Queue: queue,
		Item:  item,
		Count: 1,
	})
}

func ActionProduceVehicle(env RuleEnv, conn CommandSender) error {
	item := env.BestBuildableVehicle()
	if item == "" {
		return nil
	}
	slog.Debug("producing vehicle", "item", item)
	return sendProduce(env, conn, QueueVehicle, item)
}

func ActionProduceSpecialistInfantry(env RuleEnv, conn CommandSender) error {
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

func ActionProduceAircraft(env RuleEnv, conn CommandSender) error {
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

func ActionProduceShip(env RuleEnv, conn CommandSender) error {
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

func ActionProduceDefense(env RuleEnv, conn CommandSender) error {
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

func ActionProduceAADefense(env RuleEnv, conn CommandSender) error {
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

func ActionProduceGapGenerator(env RuleEnv, conn CommandSender) error {
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

func ActionProduceTechCenter(env RuleEnv, conn CommandSender) error {
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

func ActionProduceFlameTower(env RuleEnv, conn CommandSender) error {
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

func ActionProduceTeslaCoil(env RuleEnv, conn CommandSender) error {
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

func ActionProduceHeavyVehicle(env RuleEnv, conn CommandSender) error {
	// Affordable first, then merely buildable. As SOVIET the heavy_tank role is
	// a 2000-credit mammoth and the 1150 heavy tank sits behind it under
	// medium_tank, so taking the first BUILDABLE one parked every order on the
	// mammoth: 69 fires of this rule in game 192 and no tank delivered.
	for _, resolve := range []func(string) string{env.AffordableType, env.BuildableType} {
		for _, role := range []string{"heavy_tank", "medium_tank"} {
			if item := resolve(role); item != "" {
				slog.Debug("producing heavy vehicle", "item", item)
				return sendProduce(env, conn, QueueVehicle, item)
			}
		}
	}
	return nil
}

func ActionProduceScoutVehicle(env RuleEnv, conn CommandSender) error {
	item := env.BuildableType("ranger")
	if item == "" {
		// Soviets don't have rangers — use a light tank as scout.
		item = env.BuildableType("light_tank")
	}
	if item == "" {
		return nil
	}
	slog.Debug("producing scout vehicle", "item", item)
	return sendProduce(env, conn, QueueVehicle, item)
}

func ActionProduceSiegeVehicle(env RuleEnv, conn CommandSender) error {
	for _, role := range []string{"artillery", "v2_launcher"} {
		if item := env.BuildableType(role); item != "" {
			slog.Debug("producing siege vehicle", "item", item)
			return sendProduce(env, conn, QueueVehicle, item)
		}
	}
	return nil
}

func ActionProduceBasicAircraft(env RuleEnv, conn CommandSender) error {
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

func ActionProduceRocketSoldier(env RuleEnv, conn CommandSender) error {
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

func ActionProduceAdvancedAircraft(env RuleEnv, conn CommandSender) error {
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

func ActionProduceAdvancedPower(env RuleEnv, conn CommandSender) error {
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

func ActionProduceOreSilo(env RuleEnv, conn CommandSender) error {
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

func ActionProduceAdvancedShip(env RuleEnv, conn CommandSender) error {
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

func ActionProduceEngineer(env RuleEnv, conn CommandSender) error {
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

func ActionProduceHarvester(env RuleEnv, conn CommandSender) error {
	slog.Debug("producing harvester")
	return sendProduce(env, conn, QueueVehicle, Harvester)
}

// harvestResend throttles repeat harvest orders. A harvester idle for a single
// tick (waiting to dock, between trips) would otherwise get a fresh order that
// cancels its pathing and strands it beside the refinery.
const harvestResend = 120

func ActionProduceMissileSilo(env RuleEnv, conn CommandSender) error {
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

func ActionProduceIronCurtain(env RuleEnv, conn CommandSender) error {
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

func ActionProduceFlakTruck(env RuleEnv, conn CommandSender) error {
	item := env.BuildableType("flak_truck")
	if item == "" {
		return nil
	}
	slog.Debug("producing flak truck", "item", item)
	return sendProduce(env, conn, QueueVehicle, item)
}

func ActionProduceGunboat(env RuleEnv, conn CommandSender) error {
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

func ActionProduceAPC(env RuleEnv, conn CommandSender) error {
	item := env.BuildableType("apc")
	if item == "" {
		item = env.BuildableType("ranger")
	}
	if item == "" {
		return nil
	}
	slog.Debug("producing transport", "item", item)
	return sendProduce(env, conn, QueueVehicle, item)
}

func ActionProduceGrenadier(env RuleEnv, conn CommandSender) error {
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

func ActionProduceAttackDog(env RuleEnv, conn CommandSender) error {
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

func ActionProduceSpy(env RuleEnv, conn CommandSender) error {
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

func ActionProduceMADTank(env RuleEnv, conn CommandSender) error {
	item := env.BuildableType("mad_tank")
	if item == "" {
		return nil
	}
	slog.Debug("producing MAD tank", "item", item)
	return sendProduce(env, conn, QueueVehicle, item)
}

func ActionProduceMinelayer(env RuleEnv, conn CommandSender) error {
	item := env.BuildableType("minelayer")
	if item == "" {
		return nil
	}
	slog.Debug("producing minelayer", "item", item)
	return sendProduce(env, conn, QueueVehicle, item)
}

func ActionProduceKennel(env RuleEnv, conn CommandSender) error {
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
