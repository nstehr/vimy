package rules

import (
	"github.com/expr-lang/expr/vm"
	"github.com/nstehr/vimy/vimy-core/ipc"
)

// ActionFunc executes a rule's side effects (sending commands via conn).
type ActionFunc func(env RuleEnv, conn *ipc.Connection) error

// Rule pairs a compiled boolean expression with an action to fire when true.
type Rule struct {
	Name         string       // human-readable identifier
	Priority     int          // higher = evaluated first
	Category     string       // grouping for exclusive semantics
	Exclusive    bool         // if true, blocks lower-priority rules in same category
	ConditionSrc string       // expr source (preserved for serialization)
	program      *vm.Program  // compiled bytecode
	Action       ActionFunc
}
