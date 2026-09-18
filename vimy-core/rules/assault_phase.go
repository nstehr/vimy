package rules

import (
	"log/slog"
	"math"
)

// The base assault is already a state machine; it has just never said so. Its
// state lives in four memory maps — squadAttackState.Attacking, the hunt step,
// and two order throttles — and the transitions are implicit in nested
// conditionals inside SquadAttackKnownBase.
//
// That opacity has cost real mistakes. A reset added for the approach axis
// would silently have undone the widened hunt. A fix to target selection landed
// on a branch that never runs. Neither was visible from any log, because
// nothing records which phase a squad is in.
//
// Naming the phases does not change behaviour. It makes the machine legible
// before anyone decides whether its sequencing is wrong — which is the order
// this project has repeatedly learned to work in.
type assaultPhase string

const (
	// Waiting for enough of the squad to be together to be worth committing.
	phaseRally assaultPhase = "rally"
	// Walking in, possibly via an axis chosen to skirt known defenses.
	phaseApproach assaultPhase = "approach"
	// Close enough to shoot a structure, and doing so.
	phaseStrike assaultPhase = "strike"
	// Arrived and found nothing; searching outward in rings.
	phaseHunt assaultPhase = "hunt"
)

// assaultPhaseEntry is what a squad was doing, and since when.
type assaultPhaseEntry struct {
	Phase assaultPhase
	Since int
	// What the strike phase is shooting at, empty otherwise.
	Target string
}

// recordAssaultPhase notes the squad's phase, logging only transitions — a
// phase that holds for thirty thousand ticks should say so once, not thirty
// thousand times.
func recordAssaultPhase(env RuleEnv, squad string, phase assaultPhase, target string) {
	phases := memoryMap[string, assaultPhaseEntry](env.Memory, "assaultPhase")
	prev, seen := phases[squad]
	if seen && prev.Phase == phase && prev.Target == target {
		return
	}
	held := 0
	if seen {
		held = env.State.Tick - prev.Since
	}
	phases[squad] = assaultPhaseEntry{Phase: phase, Since: env.State.Tick, Target: target}
	slog.Info("assault phase", "squad", squad, "phase", phase,
		"target", target, "tick", env.State.Tick, "previous", prev.Phase, "held_ticks", held)
}

// AssaultPhase reports a squad's current phase, for the dashboard and for
// anyone asking why a squad has been walking for twenty thousand ticks.
func (e RuleEnv) AssaultPhase(squad string) string {
	phases := memoryMap[string, assaultPhaseEntry](e.Memory, "assaultPhase")
	if entry, ok := phases[squad]; ok {
		return string(entry.Phase)
	}
	return ""
}

// Why a squad that was told to attack did not shoot a building.
//
// Game 131 was the first in which the army actually went out: it formed 76
// squads, issued 121 base attacks, and started dying forward rather than at
// home. It also made 34 assault-phase transitions and reached `strike` ZERO
// times, destroying no buildings for the fifth game running.
//
// Four separate things can stop it, with four different fixes, and the phase
// log cannot tell them apart — it records where the squad got to, not what
// turned it back. Counting them was the lesson of the harvester tracker: a
// mechanism guessed at here was wrong once already this session.
const (
	// The squad was too strung out, so it was sent to re-gather and never
	// looked for a target at all.
	StrikeBlockedUnclumped = "unclumped"
	// Nothing of theirs was visible from the squad to score, and the squad had
	// reached the base it remembers. The squad is standing where the enemy is
	// supposed to be and can see nothing: either the memory is wrong, or
	// targeting is blind to buildings it has already scouted. Game 132 hit
	// no-target in 23 of 27 attempts and this is the half that matters.
	StrikeBlockedBlindAtBase = "blind-at-base"
	// Nothing visible, and the squad is still a walk from the base. Expected,
	// and not a defect: it has not arrived yet. Separated from the above
	// because the two have nothing to do with each other and game 132 could
	// not tell them apart — its squad was disengaging under a 5x army deficit,
	// so most of its no-targets may simply have been "nowhere near them".
	StrikeBlockedNoTargetEnRoute = "no-target-en-route"
	// Something was visible and the best of it was a unit, not a building.
	// Mobile units score 1.0 against a building's 3 to 12, so this means no
	// building was in view at all rather than a unit outranking one.
	StrikeBlockedNotBuilding = "not-building"
	// A building was the best target and the squad was still too far from it.
	StrikeBlockedOutOfReach = "out-of-reach"
)

// recordStrikeBlocked tallies one reason a strike did not happen.
//
// Counted per squad per reason, in memory, and read out at game end. Cheap
// enough to run on every evaluation, which matters: the interesting case is
// the one that happens thousands of times.
func recordStrikeBlocked(env RuleEnv, reason string) {
	counts := memoryMap[string, int](env.Memory, "strikeBlocked")
	counts[reason]++
}

// StrikeBlockers reports why strikes did not happen, by reason.
func (e RuleEnv) StrikeBlockers() map[string]int {
	return memoryMap[string, int](e.Memory, "strikeBlocked")
}

// The shape of a squad at the moment it was told to re-gather.
//
// Games 132 and 133 put 110 of 165 failed strikes on `unclumped`, and zero on
// every other cause. So the squad is never assembling — but "the radius is too
// tight" and "the rally cannot reach most of the squad" are different faults
// with different fixes, and the counter cannot tell them apart.
//
// The suspicion is the second. squadIdleActorIDs sends the rally only to IDLE
// members, SquadClumped counts ALL of them, and Idle means actor.IsIdle —
// "has no current order". So issuing the rally makes its recipients non-idle
// and drops them from the next one, while they still count against the 80%.
// If that is right, the idle count here will be a small fraction of members,
// persistently.
//
// Three numbers decide it:
//
//	members ~= idle          the rally reaches everyone; the radius is the fault
//	idle much lower          the rally cannot reach the squad; the predicate is
//	spread very large        the squad is scattered, not merely untidy
type rallyShape struct {
	Rallies    int
	MembersSum int
	IdleSum    int
	SpreadSum  int
}

// recordRallyShape notes one rally: how many members the squad had, how many
// of them could actually be commanded, and how far the furthest had strayed
// from the centroid.
func recordRallyShape(env RuleEnv, members, idle, spread int) {
	s, _ := env.Memory["rallyShape"].(*rallyShape)
	if s == nil {
		s = &rallyShape{}
		env.Memory["rallyShape"] = s
	}
	s.Rallies++
	s.MembersSum += members
	s.IdleSum += idle
	s.SpreadSum += spread
}

// squadSpread is the distance from the centroid to the furthest member, in
// cells. Max rather than mean: the 80% rule is about stragglers, and a mean
// hides the one unit holding the whole squad back.
func squadSpread(env RuleEnv, name string, cx, cy int) (members, spread int) {
	squads := getSquads(env.Memory)
	sq, ok := squads[name]
	if !ok {
		return 0, 0
	}
	ids := make(map[int]bool, len(sq.UnitIDs))
	for _, id := range sq.UnitIDs {
		ids[id] = true
	}
	worst := 0
	for _, u := range env.State.Units {
		if !ids[u.ID] {
			continue
		}
		members++
		dx, dy := u.X-cx, u.Y-cy
		if d := dx*dx + dy*dy; d > worst {
			worst = d
		}
	}
	return members, int(math.Sqrt(float64(worst)))
}
