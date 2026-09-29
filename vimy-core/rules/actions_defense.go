package rules

import (
	"log/slog"
	"math"
	"slices"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

func SquadDefend(name string) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		enemy := env.BestGroundTarget()
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
		slog.Debug("squad defending", "squad", name, "count", len(ids), "target", enemy.ID)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        enemy.X,
			Y:        enemy.Y,
		})
	}
}

// SquadGuardHarvesters sends a squad to a threatened harvester.
//
// Every other defensive action gates on the base being attacked, so a harvester
// shot at a distant ore patch summoned nobody and the only response was to run
// it home and abandon the ore.
//
// Attack-move, not move: the point is to engage what is shooting it.
func SquadGuardHarvesters(name string, dangerPct float64) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		threatened := env.HarvestersInDanger(dangerPct)
		if len(threatened) == 0 {
			return nil
		}
		ids := squadCommittableActorIDs(env, name)
		if len(ids) == 0 {
			return nil
		}

		// Nearest first, so a guard doesn't cross the map past a closer
		// harvester in the same trouble.
		target := threatened[0]
		if cx, cy, ok := squadCentroid(env, name); ok {
			best := math.MaxFloat64
			for _, h := range threatened {
				dx, dy := float64(h.X-cx), float64(h.Y-cy)
				if d := dx*dx + dy*dy; d < best {
					best, target = d, h
				}
			}
		}

		// Re-issue only on a target change or a stale order; anything else
		// cancels the in-flight path and the squad never arrives.
		type guardOrder struct {
			Target int
			Tick   int
		}
		orders := memoryMap[string, guardOrder](env.Memory, "squadGuardOrder")
		if prev, ok := orders[name]; ok &&
			prev.Target == target.ID && env.State.Tick-prev.Tick < guardResendTicks {
			return nil
		}
		orders[name] = guardOrder{Target: target.ID, Tick: env.State.Tick}

		slog.Debug("squad guarding harvester",
			"squad", name, "count", len(ids), "harvester", target.ID)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids,
			X:        target.X,
			Y:        target.Y,
		})
	}
}

// ActionScrambleToHarvesters answers a raid from the whole army, not from a
// standing detachment.
//
// The built-in AI lists harv first in ProtectionTypes and responds out of its
// general squad pool, paying nothing until something is actually attacked.
// Vimy pre-committed a four-unit harvester-guard squad — reserved so "the
// attack rules cannot poach it back" — to cover six harvesters at separate ore
// patches, and the flee counts say it is not enough: 26 flee events in the one
// win against 97 and 126 in the two losses either side of it.
//
// Units are held briefly rather than reassigned, so the squad they came from
// reclaims them when the hold lapses. That is the AI's "return to the pool"
// without a second bookkeeping path.
func ScrambleToHarvesters(dangerPct float64, maxDefenders int) ActionFunc {
	return func(env RuleEnv, conn CommandSender) error {
		danger := env.HarvestersInDanger(dangerPct)
		if len(danger) == 0 {
			return nil
		}
		held := memoryMap[int, int](env.Memory, "disengagedUntil")
		sent := memoryMap[int, scoutMoveEntry](env.Memory, "harvesterScrambleSent")

		var pool []model.Unit
		for _, u := range env.State.Units {
			if !IsCombatUnit(u.Type) || isAircraft(u) || isNaval(u) {
				continue
			}
			pool = append(pool, u)
		}
		// The same escalation the base defence uses, and for the same reason.
		// This rule was written to pull the NEAREST units, squad members
		// included — and a squad marching out across its own territory IS the
		// nearest force to a raided harvester. It matched 1320 times in game
		// 144 and acted 192, against 320 attack acts, holding each unit it
		// took for harvesterScrambleHold ticks, during which the assault
		// cannot command it. That is most of the third of itself a squad lost
		// between setting out and arriving: 3.0 members far from the target,
		// 2.4 at mid range, 2.0 on arrival.
		//
		// Harvester harassment is continuous, so this is a rule written for an
		// exception firing as the weather. It now takes the garrison and the
		// unassigned first and only reaches into an offensive when there is
		// genuinely nothing else — which means harvesters will sometimes die
		// that used to be saved. They are 48 to 65 percent productive and the
		// economy has not been the binding constraint for several games; the
		// assault has never once landed a strike.
		if free := withoutAttackSquads(env, pool); len(free) > 0 {
			pool = free
		}
		if len(pool) == 0 {
			return nil
		}

		// Which raid to answer, chosen by what this pool can actually reach.
		//
		// This was `danger[0]` under a comment claiming "nearest first", and
		// the comment described the DEFENDER sort below, not the target.
		// HarvestersInDanger walks State.Units and appends, so danger[0] is
		// whichever endangered harvester happens to hold the lowest index --
		// arbitrary, and stable across ticks, so the same one is answered every
		// time while the rest are never answered at all.
		//
		// That is not a rare case. Game 195 sampled 225 states with a harvester
		// in danger and 172 of them had THREE OR MORE at once, up to seven,
		// because the raiding is spread across ore patches rather than aimed at
		// one. Sending every defender to an arbitrary one of seven is close to
		// sending them nowhere: the harvester count sat at 5 against the two
		// wins' 10, and rebuild-harvester fired 43 times.
		//
		// Concentrating the whole pool on ONE harvester stays right -- splitting
		// eight defenders across seven raids delivers one unit per raid, which
		// is a donation. The choice of which is what changes.
		target := danger[0]
		bestReach := -1
		for _, h := range danger {
			reach := -1
			for _, u := range pool {
				d := (u.X-h.X)*(u.X-h.X) + (u.Y-h.Y)*(u.Y-h.Y)
				if reach < 0 || d < reach {
					reach = d
				}
			}
			if reach >= 0 && (bestReach < 0 || reach < bestReach) {
				bestReach, target = reach, h
			}
		}

		distSq := func(u model.Unit) int {
			return (u.X-target.X)*(u.X-target.X) + (u.Y-target.Y)*(u.Y-target.Y)
		}
		slices.SortStableFunc(pool, func(a, b model.Unit) int { return distSq(a) - distSq(b) })
		if len(pool) > maxDefenders {
			pool = pool[:maxDefenders]
		}
		if len(pool) == 0 {
			return nil
		}

		var ids []uint32
		for _, u := range pool {
			if prev, ok := sent[u.ID]; ok && env.State.Tick-prev.Tick < harvesterScrambleResend {
				continue
			}
			sent[u.ID] = scoutMoveEntry{Tick: env.State.Tick, X: target.X, Y: target.Y}
			held[u.ID] = env.State.Tick + harvesterScrambleHold
			ids = append(ids, uint32(u.ID))
		}
		if len(ids) == 0 {
			return nil
		}
		slog.Debug("scrambling to raided harvester", "count", len(ids), "harvester", target.ID, "x", target.X, "y", target.Y)
		markEffect(env)
		return conn.Send(ipc.TypeAttackMove, ipc.AttackMoveCommand{
			ActorIDs: ids, X: target.X, Y: target.Y,
		})
	}
}
