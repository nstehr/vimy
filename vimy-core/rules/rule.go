package rules

import (
	"github.com/expr-lang/expr/vm"
)

// ActionFunc sends commands to the OpenRA mod when a rule's condition is true.
type ActionFunc func(env RuleEnv, conn CommandSender) error

// Rule is a condition → action pair, the atomic unit of AI behavior. Category
// and Exclusive are what stop two rules issuing conflicting orders on one
// production queue.
type Rule struct {
	Name      string // human-readable identifier
	Priority  int    // higher = evaluated first
	Category  string // grouping for exclusive semantics
	Exclusive bool   // if true, blocks lower-priority rules in same category
	// Share rations an exclusive category: the rule may win it at most once in
	// every Share wins. Zero or one means unrationed, which is every rule today.
	//
	// Exclusive categories are a STRICT-priority scheduler, so the loser does
	// not build later, it does not build at all. vimy-l6l counted the cost
	// against the engine's own counters in game 127: produce-scout-vehicle
	// preempted 216 times and never run, produce-flak-truck 216, mad-tank 95,
	// build-ore-silo 66, build-kennel 45. produce-siege-vehicle has the same
	// problem from the other side -- it sits ABOVE produce-vehicle and is always
	// under its cap, so it took every slot, and a 3:1 ratio against live tank
	// count was added to hold it back. That ratio is a scheduling constraint
	// wearing a composition constraint's clothes, and both settings of it have
	// failed: 3:1 starves siege (artillery peaks at 2), 1:1 ran game 187's
	// treadmill at 4.10 credits lost per credit killed.
	//
	// Share makes the scheduler weighted instead of strict, which is what those
	// ratios were approximating with the wrong variable. It is deliberately
	// counted in category WINS rather than ticks: the contested resource is the
	// production slot, not time, so a rationed rule yields the same share of
	// slots whatever the game's tempo.
	Share        int
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
