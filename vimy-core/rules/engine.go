package rules

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

// Engine runs compiled rules against game state each tick.
// Rules fire in priority order; exclusive rules block lower-priority rules
// in the same category, preventing conflicting orders on the same queue.
type Engine struct {
	mu      sync.RWMutex
	rules   []*Rule
	Memory  map[string]any
	memMu   sync.Mutex // guards all reads/writes to Memory
	Terrain *model.TerrainGrid
	prefs   UnitPreferences
}

// NewEngine compiles all rule conditions into expr bytecode and sorts by priority.
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

// Evaluate runs all rules against the current game state.
func (e *Engine) Evaluate(gs model.GameState, faction string, conn *ipc.Connection) error {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	e.memMu.Lock()
	defer e.memMu.Unlock()

	env := RuleEnv{State: gs, Faction: faction, Memory: e.Memory, Terrain: e.Terrain, Preferences: e.prefs}
	updateIntel(env)
	updateBuiltRoles(env)
	updateSquads(env)
	logMilitaryDiagnostics(env)
	fired := make(map[string]bool) // category → exclusive rule already fired

	anyFired := false
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

		anyFired = true
		slog.Debug("rule fired", "rule", r.Name, "priority", r.Priority, "category", r.Category)

		if err := r.Action(env, conn); err != nil {
			slog.Error("rule action error", "rule", r.Name, "error", err)
		}

		if r.Exclusive {
			fired[r.Category] = true
		}
	}

	if !anyFired {
		logIdleDiagnostics(gs)
	}

	return nil
}

// Swap atomically replaces the rule set (called by the strategist when the LLM
// generates a new doctrine). Compiles first; if compilation fails the old rules
// remain active. Squads are cleared because the new rules may define different
// squad names and sizes.
func (e *Engine) Swap(newRules []*Rule) error {
	compiled, err := compileRules(newRules)
	if err != nil {
		return err
	}
	names := make([]string, len(compiled))
	for i, r := range compiled {
		names[i] = r.Name
	}
	e.mu.Lock()
	e.rules = compiled
	e.mu.Unlock()

	e.memMu.Lock()
	delete(e.Memory, "squads")
	e.memMu.Unlock()
	slog.Info("rule set swapped", "count", len(compiled), "rules", names)
	return nil
}

// LockMemory acquires the memory mutex. Callers must pair with UnlockMemory.
// Used by the strategist to safely read Memory from a background goroutine.
func (e *Engine) LockMemory()   { e.memMu.Lock() }
func (e *Engine) UnlockMemory() { e.memMu.Unlock() }

// SetTerrain stores the coarse terrain grid received during the hello handshake.
func (e *Engine) SetTerrain(grid *model.TerrainGrid) {
	e.mu.Lock()
	e.Terrain = grid
	e.mu.Unlock()
	slog.Info("terrain grid set", "cols", grid.Cols, "rows", grid.Rows, "cellW", grid.CellW, "cellH", grid.CellH)
}

// SetPreferences stores per-unit-type preferences from the LLM doctrine.
func (e *Engine) SetPreferences(p UnitPreferences) {
	e.mu.Lock()
	e.prefs = p
	e.mu.Unlock()
}

// logIdleDiagnostics helps debug "why isn't the AI doing anything?" —
// dumps queue state when zero rules fire. Throttled to avoid log spam.
var lastDiagTick int

func logIdleDiagnostics(gs model.GameState) {
	if gs.Tick-lastDiagTick < 100 {
		return
	}
	lastDiagTick = gs.Tick

	for _, pq := range gs.ProductionQueues {
		slog.Warn("idle diagnostics",
			"queue", pq.Type,
			"busy", pq.CurrentItem != "" && pq.CurrentProgress < 100,
			"ready", pq.CurrentItem != "" && pq.CurrentProgress >= 100,
			"buildable", strings.Join(pq.Buildable, ","),
		)
	}
	slog.Warn("idle diagnostics",
		"cash", gs.Player.Cash,
		"resources", gs.Player.Resources,
		"powerProvided", gs.Player.PowerProvided,
		"powerDrained", gs.Player.PowerDrained,
		"powerState", gs.Player.PowerState,
	)
}

// logMilitaryDiagnostics helps debug "why doesn't the AI attack?" — fires
// every 100 ticks regardless of rule activity.
var lastMilitaryDiagTick int

func logMilitaryDiagnostics(env RuleEnv) {
	if env.State.Tick-lastMilitaryDiagTick < 100 {
		return
	}
	lastMilitaryDiagTick = env.State.Tick

	totalUnits := len(env.State.Units)
	idleGround := len(env.IdleGroundUnits())
	idleAir := len(env.IdleCombatAircraft())
	idleNaval := len(env.IdleNavalUnits())
	enemiesVisible := env.EnemiesVisible()
	hasIntel := env.HasEnemyIntel()

	slog.Info("military diagnostics",
		"totalUnits", totalUnits,
		"idleGround", idleGround,
		"idleCombatAir", idleAir,
		"idleNaval", idleNaval,
		"enemiesVisible", enemiesVisible,
		"hasEnemyIntel", hasIntel,
	)
}

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
