package rules

import (
	"log/slog"
	"math"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

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
func ActionLayMines(env RuleEnv, conn CommandSender) error {
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
