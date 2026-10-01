package rules

import (
	"log/slog"
	"math"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
)

// committableGround counts the combat ground units this squad could hold: every
// one not already rostered to a DIFFERENT squad, whether or not it is idle.
//
// Not filtered on Idle, and not filtered on the pool. Idle means "has no
// current order", so a unit doing its job is invisible to it — the same
// predicate that made squads uncommandable, readiness zero while moving, and
// the base alarm permanent. And the pool is what can be ADDED this instant,
// which is the wrong question for a target.
//
// Units in another squad are excluded because that is where the garrison
// lives: ground-defense absorbed 3604 defence acts on its own in game 149
// without the offensive ever being touched. So "not in another squad" already
// means "not needed at home".
func committableGround(env RuleEnv, ownName string) int {
	claimed := make(map[int]bool)
	for name, sq := range getSquads(env.Memory) {
		if name == ownName {
			continue
		}
		for _, id := range sq.UnitIDs {
			claimed[id] = true
		}
	}
	n := 0
	for _, u := range env.State.Units {
		if !IsCombatUnit(u.Type) || isAircraft(u) || isNaval(u) || claimed[u.ID] {
			continue
		}
		n++
	}
	return n
}

// committableAir is every combat aircraft not rostered to another squad.
//
// The air counterpart of committableGround, and needed because the two squads
// have opposite problems. A ground army is bigger than the doctrine's constant,
// so squadTarget treats that constant as a floor and lets the target grow. An
// air fleet is smaller: game 185 asked for a group of 1 to 4, mean 2.7, against
// 2 to 4 combat aircraft alive at any moment.
func committableAir(env RuleEnv, ownName string) int {
	claimed := make(map[int]bool)
	for name, sq := range getSquads(env.Memory) {
		if name == ownName {
			continue
		}
		for _, id := range sq.UnitIDs {
			claimed[id] = true
		}
	}
	n := 0
	for _, u := range env.State.Units {
		if !IsCombatUnit(u.Type) || !isAircraft(u) || claimed[u.ID] {
			continue
		}
		n++
	}
	return n
}

// squadAirTarget is how big the air squad should be: the fleet that exists,
// never more than the doctrine asked for, and never one aircraft unless the
// doctrine asked for one.
//
// squadTarget's max(committable, floor) is wrong here and pins the target above
// the fleet. squad-air-attack is gated on present/TargetSize >= activation(),
// so with a constant target of 3, two living aircraft score 0.67 against a
// commit_ratio of 0.70 and the gate never opens. Game 185: squad-air-attack
// fired 125 times, stopped at tick 34790 of 97090, and neither it nor
// squad-air-attack-known-base fired again for the remaining 62000 ticks, while
// squad-air-reengage kept going to 79790 because it asks squad-idle-count > 0
// instead. Aircraft die constantly, so a constant target means one loss ends
// the air arm for the game.
//
// The floor of two is what is left of commit_ratio for air: a lone helicopter
// sent at a base is the piecemeal attack the ratio exists to prevent, and it
// stays refused. Above two the doctrine's number becomes a ceiling rather than
// a floor, because asking a three-aircraft fleet to muster four is asking for
// nothing.
func squadAirTarget(env RuleEnv, name string, want int) int {
	target := committableAir(env, name)
	if target > want {
		target = want
	}
	if floor := min(want, 2); target < floor {
		target = floor
	}
	return target
}

// squadTarget is how big this squad should be: what the doctrine asks for as a
// FLOOR, and the committable force as the actual number.
//
// The doctrine's ground_attack_group_size is a constant — 5 or 6 in every game
// the strategist has written — while the army runs from 7 combat units to 27
// at peak. A constant target is therefore a smaller and smaller fraction of
// the army as the army grows, and TargetSize was set once at formation and
// never revisited, so it could not even track the army it was formed from.
//
// Game 150 is what that costs. The squad formed at a full 6 with all 6
// commandable, held formation, and reached 0.143 of the map diagonal from the
// enemy base — the closest any assault has come. What arrived was two units,
// neither of them together, against 16 rocket soldiers and 10 APCs. It arrived
// as a pair because it was only ever six.
//
// Recomputed on every call, so the target grows with the army rather than
// freezing at whatever existed the moment the squad formed.
// squadAssaulting reports whether the squad is close enough to the enemy base
// that taking reinforcements would reshuffle a formation already in contact.
//
// This replaces a fixed join radius, which was the wrong question. A radius of
// twice the rally distance did stop the game 155 treadmill — a unit fresh off
// the war factory joining a squad at the enemy base, resetting spread to 50-odd
// every time the clump gate came within reach — but it also stranded everyone
// at home the moment the squad left. Game 159 sat on 44 idle units with a squad
// frozen at 6 for thousands of ticks: form-ground-attack fired 1038 times and
// was refused every candidate, while a fresh squad could not form either
// because one already existed. That is the same failure as freezing
// recruitment, reached from the other side.
//
// A rifleman walking to catch up with a squad crossing open ground costs
// nothing. One joining a squad already at the gate is what wrecks the assault.
// So the question is not how far the joiner is, it is whether the squad is
// still travelling — and withinStrikeReach already draws that line.
func squadAssaulting(env RuleEnv, name string) bool {
	cx, cy, ok := squadCentroid(env, name)
	if !ok {
		return false
	}
	base := env.NearestEnemyBase()
	if base == nil {
		return false
	}
	return withinStrikeReach(env, cx, cy, base.X, base.Y)
}

// squadCommitted reports whether the squad's assault is still live: committed to
// a target, and refreshed by an assault rule within the commitment TTL.
//
// This is the difference between a formation in contact and a remnant standing
// at the gate, and without it the two are indistinguishable by position -- which
// deadlocked game 194 from tick 18360 to the end.
//
// The loop: the squad decayed to 2 members at the enemy base, so squadAssaulting
// was true and reinforcement was refused. TargetSize is recomputed from
// committableGround on every call and set BEFORE the refusal, so as the army
// grew at home the target climbed -- 68 ground units at tick 16000, 93 at 24000,
// 86 of them idle. SquadReadyRatio is living members over TargetSize, so it fell
// to 2/93, far under the doctrine's commit_ratio of 0.70, and
// squad-attack-known-base could not fire: 502 fires, the last at 18360, none in
// the remaining 6700 ticks. A squad that cannot attack does not move, so
// squadAssaulting stayed true; a squad with living members is never dissolved by
// updateSquads, so no fresh squad could form either. Every unit produced for the
// rest of the game went straight to the pool recall-stray-units walks in circles
// at home -- form-ground-attack fired 1133 times and recruited nobody.
//
// The army growing is what tightened the trap, which is the signature of a
// feedback loop rather than a threshold: the more Vimy built, the further out of
// reach its own gate went. Vimy won anyway, on two aircraft.
//
// A size threshold was the obvious fix and is wrong: reinforcement would reopen
// below some fraction of the target while the assault gate stays shut at
// activation(), leaving a second dead band between the two. Commitment has no
// such band. It is also the honest question -- the guard exists to avoid
// reshuffling a formation in contact, and a stale commitment means there is no
// contact to disturb.
func squadCommitted(env RuleEnv, name string) bool {
	st := memoryMap[string, squadAttackState](env.Memory, "squadAttackState")
	prev, ok := st[name]
	if !ok || !prev.Attacking {
		return false
	}
	return env.State.Tick-prev.LastTick <= squadAttackCommitTTL
}

func squadTarget(env RuleEnv, name string, floor int) int {
	if n := committableGround(env, name); n > floor {
		return n
	}
	return floor
}

// FormSquad only assigns unit IDs; it issues no orders. Formation and action
// are separate rules so the compiler can give each its own priority and
// condition.
func FormSquad(name, domain string, size int, role string) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
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
		// Only the offensive absorbs the army. The garrison asks for
		// lerp(2, 5, ground-defense-priority) on purpose — a handful of units
		// to answer raids — and form-defense-squad runs at roughly 475 against
		// the attack's 265, so scaling it too meant it took everything first
		// and left the assault a remnant: 35 units on the field and a squad of
		// 6 at tick 13500.
		target := size
		if strings.EqualFold(role, "attack") {
			switch domain {
			case "ground":
				target = squadTarget(env, name, size)
			case "air":
				target = squadAirTarget(env, name, size)
			}
		}
		if sq, ok := squads[name]; ok && len(sq.UnitIDs) > 0 {
			// The target tracks the army, so a squad formed when there were
			// four units keeps growing as the next twenty arrive.
			sq.TargetSize = target
			// Reinforcement: top up an existing under-strength squad.
			if len(sq.UnitIDs) >= sq.TargetSize || len(pool) == 0 {
				return nil
			}
			// But only from units that are actually WITH the squad. Game 155
			// rallied 22 times without a strike, and the rally itself was
			// working — spread fell 68, 58, 54, 50, 46, 44, 43 as the squad
			// pulled together — yet every time it neared the gate a unit just
			// built at the war factory joined from the far side of the map and
			// the spread reset to 50-odd. near plateaued at 9 while need
			// climbed 9, 10, 10, 11 with each recruit. Recruitment outran
			// convergence, so the gate could never pass.
			//
			// Refusing to recruit once COMMITTED was tried first and was too
			// blunt: this design forms small and tops up, because
			// unassigned-idle-ground runs a median of 0 and a maximum of 6, so
			// a squad that cannot grow after forming never gets past the two or
			// three it started with. Game 157 fielded squads averaging 4.3
			// members and mostly exactly 2 — the "sent as a small pair" problem
			// by a new route.
			//
			// The joiner's POSITION was always the real question. A unit at the
			// muster point costs nothing to absorb; one at the factory while the
			// squad stands at the enemy base is what wrecks the formation.
			//
			// And position alone is still not enough, because a squad can be at
			// the enemy base and not be fighting. squadCommitted is the other
			// half: only a formation whose assault rule is still firing is a
			// formation, and one whose commitment has gone stale is a remnant
			// standing where it died. Refusing THAT reinforcement is what
			// strands the entire army -- see squadCommitted for game 194.
			if squadAssaulting(env, name) && squadCommitted(env, name) {
				return nil
			}
			need := sq.TargetSize - len(sq.UnitIDs) - len(sq.Joining)
			if need <= 0 {
				return nil
			}
			add := min(need, len(pool))

			// Recruits cross together when the crossing is long.
			//
			// The dispatch always sent the whole pool, which looks like batching
			// and is not: unassigned-idle-ground runs a median of 0 and units are
			// produced one at a time, so the pool is almost always ONE. Game 205
			// measured 86 reinforcements of exactly 1.0 units each, walking a mean
			// 0.262 of the map diagonal and up to 0.636 to reach a forward squad.
			//
			// That is the leak. Game 202 lost 31 of 33 combat units between ticks
			// 24000 and 48000 with the squad NOT in transit, one or two at a time,
			// born 7 cells from home and dead 44 away having travelled 46 -- 56
			// percent of the distance between the bases. Joiners are deliberately
			// not members until they arrive, so none of it appeared in a transit
			// row and the army leaked at the rate production filled it.
			//
			// A short walk is still sent immediately: a unit joining a squad near
			// home costs nothing and waiting would only idle it. The hold has a
			// deadline because the alternative failure is the one squadCommitted
			// was written to fix -- recruits that never dispatch are recruits
			// standing at home, which is the same small squad reached from the
			// other side.
			if cx, cy, ok := squadCentroid(env, name); ok && add > 0 {
				held := memoryMap[string, int](env.Memory, "reinforceHeldSince")
				if meanJoinFraction(env, pool[:add], cx, cy) > reinforceSoloFraction {
					since, holding := held[name]
					if !holding {
						since = env.State.Tick
						held[name] = since
					}
					if add < reinforceMinGroup && env.State.Tick-since < reinforceMaxHold {
						recordReinforceHeld(env, name, len(sq.UnitIDs), add,
							env.State.Tick-since, meanJoinFraction(env, pool[:add], cx, cy))
						return nil
					}
				}
				delete(held, name)
			}

			joiners := make([]uint32, 0, add)
			for i := range add {
				sq.Joining = append(sq.Joining, pool[i].ID)
				joiners = append(joiners, uint32(pool[i].ID))
			}
			env.Memory["squads"] = squads
			markEffect(env)
			slog.Info("squad reinforced", "name", name, "dispatched", add,
				"members", len(sq.UnitIDs), "joining", len(sq.Joining), "target", sq.TargetSize)
			// Send them to the squad rather than to the objective. They become
			// members when they arrive, so until then they must not be counted
			// in cohesion and must not be walked into a fight alone.
			if cx, cy, ok := squadCentroid(env, name); ok && len(joiners) > 0 {
				recordReinforce(env, name, len(sq.UnitIDs), add, pool[:add], cx, cy)
				return sendAttackMove(env, conn, joiners, cx, cy)
			}
			return nil
		}

		// Take what the pool has. Whether that is enough to be worth forming is
		// the rule's question, not this function's — form-ground-attack asks for
		// 60% of the group size and tops up — so insisting on the full target
		// here silently forms nothing.
		if len(pool) == 0 {
			return nil
		}
		take := min(target, len(pool))
		ids := make([]int, take)
		for i := range take {
			ids[i] = pool[i].ID
		}
		squads[name] = &Squad{
			Name:       name,
			Domain:     domain,
			UnitIDs:    ids,
			Role:       role,
			TargetSize: target,
		}
		// A squad forming from nothing is a new wave, not the old one limping
		// on. Clearing the commitment reopens recruitment for the muster —
		// otherwise the first squad's state freezes every squad that follows it.
		delete(memoryMap[string, squadAttackState](env.Memory, "squadAttackState"), name)
		env.Memory["squads"] = squads
		markEffect(env)
		slog.Info("squad formed", "name", name, "domain", domain, "role", role,
			"size", take, "target", target)
		return nil
	}
}

const (
	// reinforceSoloFraction is how far a recruit may walk alone, as a fraction
	// of the map diagonal. Below it the crossing is short enough that a single
	// unit is in no more danger than the squad already is.
	reinforceSoloFraction = 0.15
	// reinforceMinGroup is how many travel together across a long crossing.
	// Three is enough to answer a scout or a single raider, which is what picks
	// off a lone rifleman; it is not an escort.
	reinforceMinGroup = 3
	// reinforceMaxHold bounds the wait, because recruits that never dispatch are
	// recruits standing at home -- the failure squadCommitted exists to prevent,
	// reached from the other side. Production delivers a unit every few hundred
	// ticks, so 1500 is several chances to reach three.
	reinforceMaxHold = 1500
)

// meanJoinFraction is how far the given units must walk to reach a point, as a
// fraction of the map diagonal.
func meanJoinFraction(env RuleEnv, units []model.Unit, cx, cy int) float64 {
	mw, mh := float64(env.State.MapWidth), float64(env.State.MapHeight)
	diag := math.Sqrt(mw*mw + mh*mh)
	if len(units) == 0 || diag == 0 {
		return 0
	}
	var sum float64
	for _, u := range units {
		sum += math.Hypot(float64(u.X-cx), float64(u.Y-cy))
	}
	return sum / float64(len(units)) / diag
}

// squadCentroid returns false for an empty or unknown squad.
func squadCentroid(env RuleEnv, name string) (int, int, bool) {
	squads := getSquads(env.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return 0, 0, false
	}
	ids := make(map[int]bool, len(sq.UnitIDs))
	for _, id := range sq.UnitIDs {
		ids[id] = true
	}
	sumX, sumY, count := 0, 0, 0
	for _, u := range env.State.Units {
		if ids[u.ID] {
			sumX += u.X
			sumY += u.Y
			count++
		}
	}
	if count == 0 {
		return 0, 0, false
	}
	return sumX / count, sumY / count, true
}

// squadCommittableActorIDs includes non-idle members. A squad already moving
// toward one threatened harvester isn't idle, so waiting for idleness means
// never answering the next raid — and harassment is continuous.
func squadCommittableActorIDs(env RuleEnv, name string) []uint32 {
	squads := getSquads(env.Memory)
	sq, ok := squads[name]
	if !ok {
		return nil
	}
	retreating := getRetreatingUnits(env.Memory)
	var ids []uint32
	for _, id := range sq.UnitIDs {
		if _, isRetreating := retreating[id]; !isRetreating {
			ids = append(ids, uint32(id))
		}
	}
	return ids
}

// squadAssaultActorIDs is every member an assault may command: in the squad,
// present, not retreating, and not inside a disengage hold.
//
// NOT filtered on Idle, and that is the whole point. Idle is actor.IsIdle —
// "has no current order" — so a squad executing the order it was just given
// does not satisfy it. Filtering assaults on idleness meant the attack could
// only ever command the members that happened to be doing nothing: game 135
// measured 1.3 of 4.0 members reachable, across 763 rallies. Two thirds of
// every squad never received the order it was then judged against by
// SquadClumped, which counts all of them — so the squad could never gather
// what it had already sent, and reached `strike` zero times in 135 phase
// transitions while razing 25 buildings on the engine's own target choices.
//
// Safe to widen because sendAttackMove already skips any actor holding an
// identical order less than attackMoveResend ticks old, so re-commanding a
// unit that is already doing the right thing sends nothing. That throttle is
// what makes this a one-line change rather than a rewrite.
//
// squadCommittableActorIDs is the same idea for harvester defence and predates
// this; it omits the disengage hold deliberately, because a withdrawal must be
// able to command units it has already pulled back. An assault must not, or
// disengaging would mean nothing.
func squadAssaultActorIDs(env RuleEnv, name string) []uint32 {
	return squadActorIDs(env, name, false)
}

func squadIdleActorIDs(env RuleEnv, name string) []uint32 {
	return squadActorIDs(env, name, true)
}

func squadActorIDs(env RuleEnv, name string, onlyIdle bool) []uint32 {
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
	held := memoryMap[int, int](env.Memory, "disengagedUntil")
	var ids []uint32
	for _, id := range sq.UnitIDs {
		_, isRetreating := retreating[id]
		// A unit that has just withdrawn is not available to the attacking
		// rules until it has had time to regroup. squadCommittableActorIDs is
		// deliberately not filtered this way: a withdrawal must still be able
		// to command units it has already pulled back.
		if until, ok := held[id]; ok {
			if env.State.Tick < until {
				continue
			}
			delete(held, id)
		}
		if isRetreating {
			continue
		}
		if onlyIdle && !idleSet[id] {
			continue
		}
		ids = append(ids, uint32(id))
	}
	return ids
}

// harvesterScrambleHold is how long a unit pulled to a raided harvester stays
// out of the attack rules' hands. Long enough to arrive and fight, short enough
// that the push is not quietly disbanded.
const harvesterScrambleHold = 400

// harvesterScrambleResend throttles re-tasking the same defender.
const harvesterScrambleResend = 150
