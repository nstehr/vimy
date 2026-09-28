package rules

import (
	"log/slog"
	"math"

	"github.com/nstehr/vimy/vimy-core/ipc"
)

// RetreatDamagedUnits uses Move, not AttackMove — a retreating unit that stops
// to fight defeats the point. Vehicles go to the service depot to auto-repair,
// everything else behind base defenses. Retreating units are marked in memory
// so focus-fire and squad-attack skip them.
func RetreatDamagedUnits(hpThreshold float64) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
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

	return func(env RuleEnv, conn CommandSender) error {
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
	return func(env RuleEnv, conn CommandSender) error {
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

// strayRecallFraction is how far from the base an unassigned idle unit has to
// be before it is fetched home, as a fraction of the map diagonal. Wider than
// the 20% NearBaseGroundUnits uses for "at home", so a unit loitering just
// outside the perimeter is left alone rather than shuttled back and forth.
const strayRecallFraction = 0.35

// strayRecallTicks throttles re-issuing a walk home that is already underway.
const strayRecallTicks = 300

// ActionRecallStrayUnits walks unassigned idle ground units back to the base.
//
// Scouting dispatches combat units to map waypoints and then stops firing on
// first contact — enemies-visible and has-enemy-intel both flip early — so
// whoever is out at that moment is never advanced again, because only the
// scouting action advances them. Nothing else collects them either:
// recall-overextended knows about squads, and squads only absorb loose units
// while under strength. Game 106 left rifles standing in the corners and the
// centre of the map for the rest of the match.
func ActionRecallStrayUnits(env RuleEnv, conn CommandSender) error {
	if len(env.State.Buildings) == 0 {
		return nil
	}
	centX, centY := env.BuildingCentroid()
	mw, mh := float64(env.State.MapWidth), float64(env.State.MapHeight)
	threshold := math.Sqrt(mw*mw+mh*mh) * strayRecallFraction
	threshSq := threshold * threshold

	// The designated scout is doing this on purpose.
	scoutID := getScoutID(env.Memory)
	sent := memoryMap[int, scoutMoveEntry](env.Memory, "strayRecallSent")

	for _, u := range env.UnassignedIdleGround() {
		if scoutID != 0 && u.ID == scoutID {
			continue
		}
		dx, dy := float64(u.X-centX), float64(u.Y-centY)
		if dx*dx+dy*dy < threshSq {
			continue
		}
		if prev, ok := sent[u.ID]; ok && env.State.Tick-prev.Tick < strayRecallTicks {
			continue
		}
		sent[u.ID] = scoutMoveEntry{Tick: env.State.Tick, X: centX, Y: centY}
		slog.Debug("recalling stray unit", "id", u.ID, "type", u.Type, "dest_x", centX, "dest_y", centY)
		if err := conn.Send(ipc.TypeMove, ipc.MoveCommand{
			ActorID: uint32(u.ID), X: centX, Y: centY,
		}); err != nil {
			return err
		}
		markEffect(env)
	}
	return nil
}

// SquadDisengage moves idle squad members back toward base centroid when
// the local threat ratio is too high (outmatched).
func SquadDisengage(name string) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
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
// focusFireReachCells is how close a target must be for concentrating on it to
// be concentration rather than a chase.
//
// bestTargetForSquad scores from the squad's position but can return anything,
// including something behind it, and sendAttack on a distant actor is a move
// order wearing an attack order's clothes. squad-focus-fire is category combat,
// so it acts ALONGSIDE the exclusive assault winner rather than competing with
// it: game 173 issued 522 attack-moves at the enemy base and 88 focus-fire
// orders at whatever scored best, and the squad sawtoothed between 0.07 and
// 0.25 of the map diagonal - roughly 23 cells forward and back - while its
// membership drained from 17 to 12. The same shape as the squad-reengage bug.
//
// Ten cells is past every infantry and tank weapon in the mod, so a target
// inside it is one the squad can already shoot without relocating.
const focusFireReachCells = 10

func SquadFocusFire(name string) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		target := bestTargetForSquad(env, name)
		if target == nil {
			return nil
		}
		// Concentrate on what the squad is already in contact with. Anything
		// further is the assault rules' business, and issuing both is what
		// makes the squad dance.
		if cx, cy, ok := squadCentroid(env, name); ok {
			dx, dy := float64(target.X-cx), float64(target.Y-cy)
			if math.Hypot(dx, dy) > focusFireReachCells {
				return nil
			}
		}
		ids := squadAssaultActorIDs(env, name)
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
	return func(env RuleEnv, conn CommandSender) error {
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
