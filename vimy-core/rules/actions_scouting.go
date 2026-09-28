package rules

import (
	"log/slog"
	"math"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

// ActionScoutWithIdleUnits farms up to 2 idle ground units for perimeter recon
// when no enemy is visible. AttackMove so scouts engage what they spot en
// route. Patrol assignments persist per scout: reissuing cancels the in-flight
// path, and a stateless version left scouts bouncing between corners.
func ActionScoutWithIdleUnits(env RuleEnv, conn CommandSender) error {
	waypoints := generateWaypoints(env.State.MapWidth, env.State.MapHeight, env.Terrain)
	if len(waypoints) == 0 {
		return nil
	}
	idle := env.IdleGroundUnits()
	if len(idle) == 0 {
		return nil
	}
	state := memoryMap[int, scoutMoveEntry](env.Memory, "idleScoutMoveSent")

	// Keep the same two units on the job. IdleGroundUnits lists whatever is
	// idle in state order, so taking the first two each time lets a freshly
	// built rifle displace a scout mid-patrol — and the displaced one is left
	// standing on its last waypoint, because only this action advances it. Six
	// firings early in game 105 left units parked in corners across the map.
	party := make([]model.Unit, 0, scoutPartySize)
	for _, u := range idle {
		if _, carrying := state[u.ID]; carrying {
			party = append(party, u)
			if len(party) == scoutPartySize {
				break
			}
		}
	}
	for _, u := range idle {
		if len(party) == scoutPartySize {
			break
		}
		if _, carrying := state[u.ID]; !carrying {
			party = append(party, u)
		}
	}

	for _, u := range party {
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
func ActionScoutPatrol(env RuleEnv, conn CommandSender) error {
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
	scoutReach := env.Policy.ScoutReachPriority
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
// scoutPartySize is how many units go out on idle-unit recon. They are the
// same units each firing: a scout only advances to its next waypoint when this
// action runs for it, so a unit that loses its place is a unit left standing.
const scoutPartySize = 2

const scoutMoveResend = 40

const scoutArriveRadius = 6.0

// lightTankScoutMinVehicles is the armour that must remain before a light tank
// is worth spending on patrol.
const lightTankScoutMinVehicles = 3

func getScoutMoveState(memory map[string]any) map[int]scoutMoveEntry {
	return memoryMap[int, scoutMoveEntry](memory, "scoutMoveSent")
}
