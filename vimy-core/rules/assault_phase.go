package rules

import "log/slog"

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
