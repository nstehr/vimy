package rules

import (
	"github.com/expr-lang/expr/vm"
	"github.com/nstehr/vimy/vimy-core/ipc"
)

// ActionFunc sends commands to the OpenRA mod when a rule's condition is true.
type ActionFunc func(env RuleEnv, conn *ipc.Connection) error

// Rule is a condition → action pair, the atomic unit of AI behavior. Category
// and Exclusive are what stop two rules issuing conflicting orders on one
// production queue.
type Rule struct {
	Name         string // human-readable identifier
	Priority     int    // higher = evaluated first
	Category     string // grouping for exclusive semantics
	Exclusive    bool   // if true, blocks lower-priority rules in same category
	ConditionSrc string // expr source (preserved for serialization)
	// Why the rule exists, when the rule set said. The condition says what it
	// tests; this says what it is for, which is what reading a game back needs.
	Because string
	// The rule as `.vy` with the doctrine applied — the form someone would edit.
	Source  string
	program *vm.Program // compiled bytecode
	Action  ActionFunc
	// How vimyc spells a factory-built action; empty for registry actions, which
	// their id names. Needed because every FormSquad(...) closure shares one code
	// pointer, so its captured arguments are unrecoverable at runtime.
	ActionSrc string
}
