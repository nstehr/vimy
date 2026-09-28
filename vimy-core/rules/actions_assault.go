package rules

import (
	"log/slog"
	"math"
	"strings"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

// huntBaseState tracks which radial position a squad is cycling through
// when hunting around an enemy base. Stored in memory per squad name.
type huntBaseState struct {
	BaseX, BaseY int
	Step         int
}

// huntOffset maps a hunt step to an offset from the base centroid: step 0 is
// the centroid, then two rings of 8 compass points at radius R and 2R. Sweeping
// inward-first catches buildings just inside fog before widening to outliers.
// huntMaxStep is where the hunt wraps back to its first ring.
//
// The rings have to be able to span the map. A razed base leaves its last
// building wherever it stood, which is not necessarily beside the centroid its
// sightings averaged out to — game 105 reduced the enemy to one outlying
// barracks and then circled the empty base site, because two rings of four
// cells could not reach it. Widening is safe: squad-attack outranks the base
// attack the moment anything is visible, so a growing search only runs while
// there is nothing in sight, and finding something ends it.
func huntMaxStep(mapDim, radius int) int {
	if radius < 1 {
		radius = 1
	}
	// Round the ring count up: truncating leaves the outermost ring short of
	// the half-map it is meant to cover.
	rings := (mapDim/2 + radius - 1) / radius
	return 8 * max(2, rings)
}

func huntOffset(step, radius int) (int, int) {
	if step <= 0 {
		return 0, 0
	}
	idx := (step - 1) % 8       // which of 8 compass points (0-7)
	ring := (step-1)/8 + 1      // which ring: 1 for steps 1-8, 2 for 9-16
	r := float64(radius * ring) // inner ring = R, outer ring = 2R
	angle := float64(idx) * 2 * math.Pi / 8
	return int(r * math.Cos(angle)), int(r * math.Sin(angle))
}

// squadAttackState records target commitment so rally-then-attack demands
// clumping only on the initial deploy. Re-checking mid-fight oscillates:
// attack, spread, regroup, attack.
type squadAttackState struct {
	TargetX, TargetY int
	Attacking        bool // false = still regrouping to centroid
	LastTick         int
}

const (
	// Long enough to finish a typical engagement, short enough that a new
	// target still gets a fresh assembly.
	squadAttackCommitTTL = 2000
	// Clumped means 80% of members within this many cells of the centroid.
	squadRallyRadius = 8
)

// bestTargetForSquad scores from the squad's own position, falling back to the
// base when the squad has no position to speak of.
func bestTargetForSquad(env RuleEnv, name string) *model.Enemy {
	if cx, cy, ok := squadCentroid(env, name); ok {
		return env.BestGroundTargetFrom(cx, cy)
	}
	return env.BestGroundTarget()
}

func SquadAttackMove(name string) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		// Scored from where the squad is, not from home. Anchoring on our own
		// base made every enemy building look distant to a squad standing in
		// front of it.
		enemy := bestTargetForSquad(env, name)
		if enemy == nil {
			enemy = env.NearestEnemy()
		}
		if enemy == nil {
			return nil
		}
		ids := squadAssaultActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}

		// Rally before committing: AttackMove to our own centroid pulls
		// stragglers up and halts the leaders, so the squad arrives together
		// rather than being fed in piecemeal.
		state := memoryMap[string, squadAttackState](env.Memory, "squadAttackState")
		prev, hasPrev := state[name]
		targetChanged := !hasPrev || prev.TargetX != enemy.X || prev.TargetY != enemy.Y
		commitStale := hasPrev && env.State.Tick-prev.LastTick > squadAttackCommitTTL
		if targetChanged || commitStale || !prev.Attacking {
			cx, cy, haveCentroid := squadCentroid(env, name)
			// Same doorstep rule as the base assault, and no centroid still
			// falls through to a direct attack.
			arrived := haveCentroid && withinStrikeReach(env, cx, cy, enemy.X, enemy.Y)
			if env.SquadClumped(name, squadRallyRadius) || arrived || !haveCentroid {
				state[name] = squadAttackState{TargetX: enemy.X, TargetY: enemy.Y, Attacking: true, LastTick: env.State.Tick}
			} else {
				state[name] = squadAttackState{TargetX: enemy.X, TargetY: enemy.Y, Attacking: false, LastTick: env.State.Tick}
				slog.Debug("squad rallying before attack", "squad", name, "count", len(ids), "target", enemy.ID, "centroid_x", cx, "centroid_y", cy)
				return sendAttackMove(env, conn, ids, cx, cy)
			}
		} else {
			prev.LastTick = env.State.Tick
			state[name] = prev
		}

		// Route around a hot defense corridor rather than attack-moving through
		// it. The waypoint helper gates on threat itself, so a clear path
		// declines and we fall through to direct routing.
		tx, ty := enemy.X, enemy.Y
		if wx, wy, ok := groundApproachWaypointFor(env, name, enemy.X, enemy.Y); ok {
			tx, ty = wx, wy
			slog.Debug("squad routing via waypoint", "squad", name, "wp_x", wx, "wp_y", wy, "target", enemy.ID)
		}

		slog.Debug("squad attack-move", "squad", name, "count", len(ids), "target", enemy.ID, "x", tx, "y", ty)
		return sendAttackMove(env, conn, ids, tx, ty)
	}
}

// SquadNudgeStragglers moves only the squad members that have gone idle, toward
// wherever the squad as a whole is already headed.
//
// squad-reengage used squad-attack-move, which is the main assault action and
// commands EVERY member. Its own note says it is there to "catch stragglers
// finishing an order while the squad presses forward", and it triggers on a
// single idle member — so one straggler anywhere in the squad redirected the
// whole army, to bestTargetForSquad rather than to the base the assault was
// marching on. It is not in the exclusive ground-attack-choice category either,
// so it acted alongside the exclusive winner rather than competing with it.
//
// Game 159 is what that looked like: squad-attack-known-base fired 210 times
// against squad-reengage's 104, and 51 units sawtoothed across 0.3 of the map
// diagonal — roughly 38 cells forward, 38 cells back — for thousands of ticks
// in front of the enemy base, never closing. Two rules steering the same army
// at two different targets.
//
// So this commands the idle members and nothing else, and it steers them at the
// squad's own centroid: the straggler's job is to rejoin, not to pick a fight
// of its own. No rally, no attack state — the assault rules own that.
func SquadNudgeStragglers(name string) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		ids := squadIdleActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}
		cx, cy, ok := squadCentroid(env, name)
		if !ok {
			return nil
		}
		slog.Debug("nudging stragglers", "squad", name, "count", len(ids), "x", cx, "y", cy)
		return sendAttackMove(env, conn, ids, cx, cy)
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

// attackOrderEntry throttles repeat Attack orders. OpenRA treats each as
// cancel-and-restart, so re-issuing pins units in place unable to close and
// fire — the same failure mode as Move and Capture.
type attackOrderEntry struct {
	Tick     int
	TargetID int
}

const attackOrderResend = 60

// sendSquadAttack puts the whole squad on one target, which is what focus fire
// means: kill the thing, then pick the next. Per-actor throttling comes from
// sendAttack.
func sendSquadAttack(env RuleEnv, conn CommandSender, ids []uint32, targetID int) error {
	for _, id := range ids {
		if err := sendAttack(env, conn, id, uint32(targetID)); err != nil {
			return err
		}
	}
	markEffect(env)
	return nil
}

// sendAttack suppresses identical re-issues within attackOrderResend ticks.
func sendAttack(env RuleEnv, conn CommandSender, actorID, targetID uint32) error {
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

// sendAttackMove batches an AttackMove across actorIDs, skipping any actor
// with an identical in-flight order. Each fresh command cancels the current
// path, so an unthrottled squad action stalls its units mid-map.
func sendAttackMove(env RuleEnv, conn CommandSender, actorIDs []uint32, x, y int) error {
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

// structureStrikeRange is how close the squad must be before it stops walking
// and starts shooting, as a fraction of the map diagonal.
const structureStrikeFraction = 0.12

// squadStructureTarget is the enemy structure worth shooting, if the squad has
// arrived somewhere it can shoot one.
//
// Scored from the squad rather than from home, so the building in front of it
// outranks a rifleman back at our own base — see BestGroundTargetFrom.
func squadStructureTarget(env RuleEnv, name string) *model.Enemy {
	cx, cy, ok := squadCentroid(env, name)
	if !ok {
		return nil
	}
	target := env.BestGroundTargetFrom(cx, cy)
	if target == nil {
		// Nothing in sight, but something may be remembered. A building that
		// went back into fog used to stop existing here, which is what
		// blind-at-base was counting: the squad standing on the enemy base with
		// nothing to shoot. Only worth using in reach - walking to a memory is
		// the assault's job, not the strike's.
		if st := env.NearestRememberedStructure(cx, cy); st != nil && withinStrikeReach(env, cx, cy, st.X, st.Y) {
			return st
		}
		// Whether it has arrived decides what this means. Standing on the
		// remembered base and seeing nothing is a targeting defect; still
		// walking is just walking.
		if base := env.NearestEnemyBase(); base != nil && withinStrikeReach(env, cx, cy, base.X, base.Y) {
			recordStrikeBlocked(env, name, StrikeBlockedBlindAtBase)
			return nil
		}
		recordStrikeBlocked(env, name, StrikeBlockedNoTargetEnRoute)
		return nil
	}
	if !IsKnownBuildingType(target.Type) {
		// A defender outscored the structures, which is not a reason to call off
		// the strike - it is a reason to ask which STRUCTURE is worth shooting.
		// This branch used to return nil, and game 180 recorded it 371 times
		// while destroying one enemy building in 34790 ticks.
		//
		// Same order as the sighted-nothing branch below: what is visible first,
		// then what is remembered, and out-of-reach is a walk rather than a
		// failure.
		if st := env.BestGroundStructureFrom(cx, cy); st != nil {
			if withinStrikeReach(env, cx, cy, st.X, st.Y) {
				return st
			}
			recordStrikeBlocked(env, name, StrikeBlockedOutOfReach)
			return nil
		}
		if st := env.NearestRememberedStructure(cx, cy); st != nil && withinStrikeReach(env, cx, cy, st.X, st.Y) {
			return st
		}
		recordStrikeBlocked(env, name, StrikeBlockedNotBuilding)
		return nil
	}
	if !withinStrikeReach(env, cx, cy, target.X, target.Y) {
		recordStrikeBlocked(env, name, StrikeBlockedOutOfReach)
		return nil // still a walk away; keep moving
	}
	return target
}

// withinStrikeReach reports whether a squad standing at cx,cy has arrived at
// tx,ty — inside structureStrikeFraction of the map diagonal. It already
// decided what a missing target means; it now also decides when a rally has
// stopped being a gathering move.
func withinStrikeReach(env RuleEnv, cx, cy, tx, ty int) bool {
	mw, mh := float64(env.State.MapWidth), float64(env.State.MapHeight)
	reach := math.Sqrt(mw*mw+mh*mh) * structureStrikeFraction
	dx, dy := float64(tx-cx), float64(ty-cy)
	return dx*dx+dy*dy <= reach*reach
}

// siegeRangeCells is each siege weapon's reach, read from the mod's own
// weapons.yaml rather than transcribed: 155mm is 20c0, SCUD is 10c0.
var siegeRangeCells = map[string]int{
	Artillery:  20, // 155mm, Allied
	V2Launcher: 10, // SCUD, Soviet
}

// maxBaseDefenceRange is the longest reach of any Red Alert base defence.
// TurretGun 6c512 is the furthest; TeslaZap is 6c0, the flame tower inherits
// ^FireWeapon at 5c0, and the pillbox chain gun is 5c0. Rounded up to 7.
const maxBaseDefenceRange = 7

// siegeStandoffCells is how far from the target a siege unit should hold: far
// enough out that no base defence reaches it, near enough that its own gun
// does. Artillery reaches 20 cells and a flame tower reaches 5, so walking the
// artillery to the base centroid alongside the riflemen throws away a threefold
// range advantage and is why the squad keeps being destroyed on entry.
//
// The margin matters because defences ring the base rather than sitting at its
// centroid, so distance-to-centroid understates distance-to-nearest-tower. Four
// cells of slack off the weapon's own range, floored at two cells beyond the
// longest defence, which for a V2 at 10c0 leaves a tight but real window.
func siegeStandoffCells(unitType string) (int, bool) {
	r, ok := siegeRangeCells[baseUnitType(unitType)]
	if !ok {
		return 0, false
	}
	stand := r - 4
	if floor := maxBaseDefenceRange + 2; stand < floor {
		stand = floor
	}
	if stand > r {
		return 0, false
	}
	return stand, true
}

func baseUnitType(t string) string {
	t = strings.ToLower(t)
	if i := strings.IndexByte(t, '.'); i >= 0 {
		t = t[:i]
	}
	return t
}

// standoffPoint is the point `cells` from the target, on the line back toward
// the squad. When the squad is already closer than that, it pulls them out.
func standoffPoint(cx, cy, tx, ty, cells int) (int, int) {
	dx, dy := float64(cx-tx), float64(cy-ty)
	d := math.Hypot(dx, dy)
	if d < 1 {
		return cx, cy
	}
	f := float64(cells) / d
	return tx + int(dx*f), ty + int(dy*f)
}

// splitSiege divides the order between units that should hold at range and
// units that should close. Returns the closers unchanged when nothing in the
// squad is a siege unit, which is the common case.
func splitSiege(env RuleEnv, ids []uint32) (closers []uint32, siege map[uint32]int) {
	typeOf := make(map[uint32]string, len(env.State.Units))
	for _, u := range env.State.Units {
		typeOf[uint32(u.ID)] = u.Type
	}
	for _, id := range ids {
		if cells, ok := siegeStandoffCells(typeOf[id]); ok {
			if siege == nil {
				siege = map[uint32]int{}
			}
			siege[id] = cells
			continue
		}
		closers = append(closers, id)
	}
	return closers, siege
}

func SquadAttackKnownBase(name string, aggression float64) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		base := env.NearestEnemyBase()
		if base == nil {
			return nil
		}
		ids := squadAssaultActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}

		// Base attacks are the worst dispersal case — the walk is long enough
		// that tanks and dogs arrive and die before the rifles catch up.
		aState := memoryMap[string, squadAttackState](env.Memory, "squadAttackState")
		prev, hasPrev := aState[name]
		targetChanged := !hasPrev || prev.TargetX != base.X || prev.TargetY != base.Y
		commitStale := hasPrev && env.State.Tick-prev.LastTick > squadAttackCommitTTL
		// A squad walking in again picks its entry again. Not on commitStale:
		// that fires every 2000 ticks of a fight already in progress, and the
		// step counter it would reset is the outward ring search, which needs
		// to keep growing to find what is left of a razed base.
		reapproach := targetChanged || !prev.Attacking
		if targetChanged || commitStale || !prev.Attacking {
			cx, cy, haveCentroid := squadCentroid(env, name)
			// The rally gathers the squad for the WALK IN, and it is spent the
			// moment the squad is on the doorstep: the else-branch attack-moves
			// the squad onto its OWN centroid, which at that range is not
			// gathering but standing still inside the base's defensive envelope,
			// re-issued every evaluation. Arriving unclumped and attacking is
			// worse than arriving clumped; arriving unclumped and holding still
			// under the guns is worse than either.
			//
			// How often this fires is another matter, and the honest answer is
			// rarely. It was justified by "game 154 marched 27 units to within
			// 17% of the enemy base", which was a misreading: on a transit row
			// `near` is the COUNT of members inside the clump radius, not a
			// distance — the distance is attrs["target_fraction"]. Read
			// properly, no squad in game 156 ever closed past 0.255 of the map
			// diagonal and the army-sized ones stopped at 0.391, all of it in
			// the Far band. The squad does not stall on the doorstep, it never
			// reaches the doorstep. This guard is correct and cheap; it is not
			// the thing keeping Vimy from striking.
			arrived := haveCentroid && withinStrikeReach(env, cx, cy, base.X, base.Y)
			if env.SquadClumped(name, squadRallyRadius) || arrived || !haveCentroid {
				aState[name] = squadAttackState{TargetX: base.X, TargetY: base.Y, Attacking: true, LastTick: env.State.Tick}
			} else {
				aState[name] = squadAttackState{TargetX: base.X, TargetY: base.Y, Attacking: false, LastTick: env.State.Tick}
				recordAssaultPhase(env, name, phaseRally, "")
				recordStrikeBlocked(env, name, StrikeBlockedUnclumped)
				// ids is the COMMANDABLE members — the roster minus whoever
				// is retreating or held — because that is exactly who
				// sendAttackMove below is given. Not the idle ones:
				// squadAssaultActorIDs passes onlyIdle=false, and calling this
				// "idle" for a year made reachable read as an idleness rate.
				// members is all of them, which is what the gate judges, and
				// near is how many of those the gate actually counted.
				members, near, _ := env.SquadClump(name, squadRallyRadius)
				_, spread := squadSpread(env, name, cx, cy)
				recordRallyShape(env, name, members, len(ids), spread, near)
				return sendAttackMove(env, conn, ids, cx, cy)
			}
		} else {
			prev.LastTick = env.State.Tick
			aState[name] = prev
		}

		// Arrived, and something of theirs is standing here: shoot it.
		//
		// Until now the squad attack-moved to a coordinate and left the choice
		// to the engine, which engages whatever wanders past — so five measured
		// games killed 184750 credits of enemy units and took zero buildings.
		// A person clicks the target: whatever is shooting back, then what
		// replaces their losses, then the rest. groundTargetValue already says
		// that — construction yard 12, tesla coil 10, turret 8, refinery 7, war
		// factory 6 — and nothing was reading it once the squad got there.
		if target := squadStructureTarget(env, name); target != nil {
			recordAssaultPhase(env, name, phaseStrike, target.Type)
			if err := sendSquadAttack(env, conn, ids, target.ID); err != nil {
				return err
			}
			aState[name] = squadAttackState{TargetX: base.X, TargetY: base.Y, Attacking: true, LastTick: env.State.Tick}
			return nil
		}

		memKey := "huntBase:" + name
		state, _ := env.Memory[memKey].(*huntBaseState)
		if state == nil {
			state = &huntBaseState{}
		}

		if state.BaseX != base.X || state.BaseY != base.Y || reapproach {
			state.BaseX = base.X
			state.BaseY = base.Y
			state.Step = 0
		}

		tx, ty := base.X, base.Y

		// Enter via a zone that skirts remembered defenses; once the squad is
		// there, later steps run at the base centroid as normal.
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

		mapDim := max(env.State.MapWidth, env.State.MapHeight)
		baseRadius := mapDim / 16
		scale := 0.25 + aggression*1.25
		radius := int(float64(baseRadius) * scale)
		if radius < 1 {
			radius = 1
		}
		maxStep := huntMaxStep(mapDim, radius)

		// A remembered building beats the search. huntOffset spirals outward
		// from the base centroid because, until enemy structure positions were
		// kept, there was nothing better to aim at: a building that went back
		// into fog stopped existing and the squad arrived to blind-at-base, 37
		// times in game 173. When something IS remembered, walk to it.
		//
		// The hunt stays as the fallback rather than being deleted. Memory can
		// be wrong - a razed base leaves stale entries until one of ours stands
		// on the site - and the ring search is what finds the last outlying
		// barracks that nothing ever sighted.
		if state.Step > 0 {
			fromX, fromY := base.X, base.Y
			if sx, sy, ok := squadCentroid(env, name); ok {
				fromX, fromY = sx, sy
			}
			st := env.NearestRememberedStructure(fromX, fromY)
			if st != nil {
				tx, ty = st.X, st.Y
				slog.Debug("squad targeting remembered structure",
					"squad", name, "type", st.Type, "x", tx, "y", ty)
			} else {
				dx, dy := huntOffset(state.Step, radius)
				tx = base.X + dx
				ty = base.Y + dy
			}

			tx = max(0, min(tx, env.State.MapWidth-1))
			ty = max(0, min(ty, env.State.MapHeight-1))

			squads := getSquads(env.Memory)
			sq := squads[name]
			if sq != nil && sq.Domain != "air" && env.Terrain != nil {
				t := env.Terrain.AtMapPos(tx, ty)
				if t != model.Land && t != model.Bridge {
					tx, ty = base.X, base.Y // fallback to centroid
				}
			}
		}

		// Suppress the step advance along with the send: otherwise the 16-step
		// hunt rotates once per tick and no step is ever reached.
		if !attackMoveHasFreshTarget(env, ids, tx, ty) {
			env.Memory[memKey] = state
			return nil
		}

		if state.Step == 0 {
			recordAssaultPhase(env, name, phaseApproach, "")
		} else {
			recordAssaultPhase(env, name, phaseHunt, "")
		}
		// Sampled here because this is the squad in transit: past the rally
		// check, committed, and being sent at a target.
		recordTransit(env, name, tx, ty)

		// Wraps to 1, not 0 — the centroid is only worth the initial approach.
		if state.Step >= maxStep {
			state.Step = 1
		} else {
			state.Step++
		}
		env.Memory[memKey] = state

		// Siege units hold at their own standoff; everyone else closes. Sending
		// the whole squad to the same point walks a 20-cell gun into a 5-cell
		// flame tower alongside the riflemen.
		closers, siege := splitSiege(env, ids)
		if len(siege) == 0 {
			return sendAttackMove(env, conn, ids, tx, ty)
		}
		cx, cy, haveCentroid := squadCentroid(env, name)
		if !haveCentroid {
			return sendAttackMove(env, conn, ids, tx, ty)
		}
		for id, cells := range siege {
			sx, sy := standoffPoint(cx, cy, tx, ty, cells)
			if err := sendAttackMove(env, conn, []uint32{id}, sx, sy); err != nil {
				return err
			}
		}
		if len(closers) == 0 {
			return nil
		}
		return sendAttackMove(env, conn, closers, tx, ty)
	}
}

// attackMoveHasFreshTarget reports whether a send to (x,y) would reach any
// actor, so callers advance internal state only when the order will land.
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
