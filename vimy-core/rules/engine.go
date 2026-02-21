package rules

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

// Engine evaluates rules against game state each tick.
type Engine struct {
	mu     sync.RWMutex
	rules  []*Rule
	Memory map[string]any
}

// NewEngine compiles all rule conditions and returns a ready engine.
func NewEngine(rules []*Rule) (*Engine, error) {
	compiled, err := compileRules(rules)
	if err != nil {
		return nil, err
	}
	return &Engine{
		rules:  compiled,
		Memory: make(map[string]any),
	}, nil
}

// Evaluate runs all rules against the game state, highest priority first.
// Exclusive rules block lower-priority rules in the same category.
func (e *Engine) Evaluate(gs model.GameState, faction string, conn *ipc.Connection) error {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	env := RuleEnv{State: gs, Faction: faction, Memory: e.Memory}
	fired := make(map[string]bool) // category → exclusive rule already fired

	for _, r := range rules {
		if fired[r.Category] {
			continue
		}

		result, err := vm.Run(r.program, env)
		if err != nil {
			slog.Warn("rule condition error", "rule", r.Name, "error", err)
			continue
		}

		match, ok := result.(bool)
		if !ok || !match {
			continue
		}

		slog.Info("rule fired", "rule", r.Name, "priority", r.Priority, "category", r.Category)

		if err := r.Action(env, conn); err != nil {
			slog.Error("rule action error", "rule", r.Name, "error", err)
		}

		if r.Exclusive {
			fired[r.Category] = true
		}
	}

	return nil
}

// Swap atomically replaces the rule set. Compiles new rules first; if compilation
// fails the old rules remain active.
func (e *Engine) Swap(newRules []*Rule) error {
	compiled, err := compileRules(newRules)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.rules = compiled
	e.mu.Unlock()
	slog.Info("rule set swapped", "count", len(compiled))
	return nil
}

// compileRules compiles all rule conditions and sorts by priority descending.
func compileRules(rules []*Rule) ([]*Rule, error) {
	for _, r := range rules {
		prog, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			return nil, fmt.Errorf("compile rule %q: %w", r.Name, err)
		}
		r.program = prog
	}
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})
	return rules, nil
}
