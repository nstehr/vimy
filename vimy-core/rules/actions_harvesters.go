package rules

import (
	"log/slog"
	"math"
	"reflect"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

type harvestEntry struct {
	Tick int
	X    int
	Y    int
}

func ActionSendIdleHarvesters(env RuleEnv, conn CommandSender) error {
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
		default:
			if x, y, ok := env.baseAnchor(); ok {
				tx, ty = x, y
			}
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
	return func(env RuleEnv, conn CommandSender) error {
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
		// The base, not a captured derrick: a harvester fleeing a raid should
		// run home, and three derricks held drag this centroid across the map.
		if x, y, ok := env.baseCentroid(); ok {
			fallbackX, fallbackY = x, y
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
