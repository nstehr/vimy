package rules

import (
	"github.com/expr-lang/expr/vm"
	"github.com/nstehr/vimy/vimy-core/ipc"
)

// ActionFunc sends commands to the OpenRA mod when a rule's condition is true.
type ActionFunc func(env RuleEnv, conn *ipc.Connection) error

// Rule is the atomic unit of AI behavior: a condition → action pair.
// The engine evaluates rules by priority and uses Category + Exclusive
// to prevent conflicting actions on the same production queue.
type Rule struct {
	Name         string      // human-readable identifier
	Priority     int         // higher = evaluated first
	Category     string      // grouping for exclusive semantics
	Exclusive    bool        // if true, blocks lower-priority rules in same category
	ConditionSrc string      // expr source (preserved for serialization)
	// Why the rule exists, when the rule set said. A condition explains what a
	// rule tests; this explains what it is for, which is what someone reading a
	// game back actually wants.
	Because string
	program      *vm.Program // compiled bytecode
	Action       ActionFunc
	// How vimyc spells this action, for actions built by a factory. Empty for
	// the rest, which are named by their ActionRegistry id.
	//
	// Needed because a closure cannot be identified at runtime: every
	// `FormSquad(...)` shares one code pointer, so the arguments it captured
	// are unrecoverable once it is built.
	ActionSrc string
}
