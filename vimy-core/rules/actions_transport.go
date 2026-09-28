package rules

import (
	"log/slog"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

func ActionLoadEngineerIntoAPC(env RuleEnv, conn CommandSender) error {
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

const apcExploreTTL = 600

const apcExploreArriveRadius = 12

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
func sendAPCMove(env RuleEnv, conn CommandSender, actorID, x, y int) error {
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

func ActionUnloadAPCNearTarget(env RuleEnv, conn CommandSender) error {
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
func ActionLoadCombatInfantry(env RuleEnv, conn CommandSender) error {
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
func ActionDeliverAssaultAPC(env RuleEnv, conn CommandSender) error {
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
