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

// RuleSummary is a read-only DTO for exposing compiled rules to the dashboard.
type RuleSummary struct {
	Name         string
	Priority     int
	Category     string
	Exclusive    bool
	ConditionSrc string
	Because      string
}

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
	bias    TargetBias

	// Per-rule firing counters for the current doctrine window. Reset by
	// FlushFiringStats when a doctrine swap occurs or the game ends.
	// Only populated when traceFirings is true.
	exporter *StateExporter

	statsMu      sync.Mutex
	traceFirings bool
	fireCounts   map[string]int
	firstTick    map[string]int
	lastTick     map[string]int
}

// RuleFiringStats captures how often a rule fired during a doctrine window
// and the tick range over which it fired.
type RuleFiringStats struct {
	FireCount int
	FirstTick int
	LastTick  int
}

// NewEngine compiles all rule conditions into expr bytecode and sorts by priority.
func NewEngine(rules []*Rule) (*Engine, error) {
	compiled, err := compileRules(rules)
	if err != nil {
		return nil, err
	}
	return &Engine{
		rules:      compiled,
		Memory:     make(map[string]any),
		fireCounts: make(map[string]int),
		firstTick:  make(map[string]int),
		lastTick:   make(map[string]int),
	}, nil
}

// SetExporter attaches a recorder for vimyc's differential corpus. Nil disables
// it, which is the default — projecting costs roughly 60x what evaluating does.
func (e *Engine) SetExporter(x *StateExporter) {
	e.mu.Lock()
	e.exporter = x
	e.mu.Unlock()
}

// Exporter returns the attached recorder, or nil.
func (e *Engine) Exporter() *StateExporter {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.exporter
}

// SetTraceFirings enables or disables per-rule firing instrumentation.
// When disabled (the default), recordFiring is a no-op and FlushFiringStats
// returns empty maps. Toggle takes effect on subsequent Evaluate calls.
func (e *Engine) SetTraceFirings(on bool) {
	e.statsMu.Lock()
	e.traceFirings = on
	e.statsMu.Unlock()
	slog.Info("rule firing trace toggled", "enabled", on)
}

// TracingEnabled reports whether firing instrumentation is currently on.
func (e *Engine) TracingEnabled() bool {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	return e.traceFirings
}

// recordFiring increments the per-rule firing counter and updates the tick
// range. No-op when tracing is disabled — one early-return per firing.
func (e *Engine) recordFiring(name string, tick int) {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	if !e.traceFirings {
		return
	}
	if _, ok := e.firstTick[name]; !ok {
		e.firstTick[name] = tick
	}
	e.lastTick[name] = tick
	e.fireCounts[name]++
}

// FlushFiringStats atomically reads and clears the current window's counters.
// Callers use this on doctrine swap (attach stats to the outgoing doctrine
// record) and at game end (attach stats to the final doctrine record).
// Returns empty when tracing is disabled.
func (e *Engine) FlushFiringStats() map[string]RuleFiringStats {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	out := make(map[string]RuleFiringStats, len(e.fireCounts))
	for name, count := range e.fireCounts {
		out[name] = RuleFiringStats{
			FireCount: count,
			FirstTick: e.firstTick[name],
			LastTick:  e.lastTick[name],
		}
	}
	e.fireCounts = make(map[string]int)
	e.firstTick = make(map[string]int)
	e.lastTick = make(map[string]int)
	return out
}

// FiringStatsSnapshot returns a non-destructive copy of the current window's
// counters. Used by the dashboard for live visibility; does NOT reset.
func (e *Engine) FiringStatsSnapshot() map[string]RuleFiringStats {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	out := make(map[string]RuleFiringStats, len(e.fireCounts))
	for name, count := range e.fireCounts {
		out[name] = RuleFiringStats{
			FireCount: count,
			FirstTick: e.firstTick[name],
			LastTick:  e.lastTick[name],
		}
	}
	return out
}

// RuleNames returns the names of all rules currently compiled into the
// engine — the "available" rule set for the current doctrine window. Used
// alongside FlushFiringStats so a coding agent can compute which rules were
// available but never fired (counterfactual).
func (e *Engine) RuleNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, len(e.rules))
	for i, r := range e.rules {
		names[i] = r.Name
	}
	return names
}

// Evaluate runs all rules against the current game state.
func (e *Engine) Evaluate(gs model.GameState, faction string, conn *ipc.Connection) error {
	e.mu.RLock()
	rules := e.rules
	exporter := e.exporter
	e.mu.RUnlock()

	e.memMu.Lock()
	defer e.memMu.Unlock()

	env := RuleEnv{State: gs, Faction: faction, Memory: e.Memory, Terrain: e.Terrain, Preferences: e.prefs, TargetBias: e.bias}
	updateIntel(env)
	updateBuiltRoles(env)
	updateSquads(env)
	updateMinelayers(env)
	designateScout(env)
	logMilitaryDiagnostics(env)
	logProductionDiagnostics(env)
	logProduceInfantryGate(env)
	logCashFlow(env)
	fired := make(map[string]bool) // category → exclusive rule already fired

	// Re-projected after each firing rather than once per evaluation: an
	// action mutates Memory, and later rules in the same tick see it.
	stateIdx := exporter.begin(env, rules)
	ruleSetID := ""
	if stateIdx >= 0 {
		ruleSetID = RuleSetID(rules)
	}

	anyFired := false
	for _, r := range rules {
		if fired[r.Category] {
			exporter.record(stateIdx, gs.Tick, ruleSetID, r.Name, false, true)
			continue
		}

		result, err := vm.Run(r.program, env)
		if err != nil {
			slog.Warn("rule condition error", "rule", r.Name, "error", err)
			continue
		}

		match, ok := result.(bool)
		exporter.record(stateIdx, gs.Tick, ruleSetID, r.Name, ok && match, false)
		if !ok || !match {
			continue
		}

		anyFired = true
		slog.Debug("rule fired", "rule", r.Name, "priority", r.Priority, "category", r.Category)
		e.recordFiring(r.Name, gs.Tick)

		if err := r.Action(env, conn); err != nil {
			slog.Error("rule action error", "rule", r.Name, "error", err)
		}
		stateIdx = exporter.refresh(stateIdx, env, rules)

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

// Reset clears all accumulated state so the engine is ready for a new game.
// The compiled rules and terrain are preserved; only per-game memory and
// firing counters are wiped.
func (e *Engine) Reset() {
	e.memMu.Lock()
	e.Memory = make(map[string]any)
	e.memMu.Unlock()

	e.mu.Lock()
	e.prefs = UnitPreferences{}
	e.bias = TargetBias{}
	e.mu.Unlock()

	e.statsMu.Lock()
	e.fireCounts = make(map[string]int)
	e.firstTick = make(map[string]int)
	e.lastTick = make(map[string]int)
	e.statsMu.Unlock()

	slog.Info("engine reset")
}

// LockMemory acquires the memory mutex. Callers must pair with UnlockMemory.
// Used by the strategist to safely read Memory from a background goroutine.
func (e *Engine) LockMemory()   { e.memMu.Lock() }
func (e *Engine) UnlockMemory() { e.memMu.Unlock() }

// Rules returns a snapshot of the current rule set as read-only summaries.
func (e *Engine) Rules() []RuleSummary {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	out := make([]RuleSummary, len(rules))
	for i, r := range rules {
		out[i] = RuleSummary{
			Name:         r.Name,
			Priority:     r.Priority,
			Category:     r.Category,
			Exclusive:    r.Exclusive,
			ConditionSrc: r.ConditionSrc,
			Because:      r.Because,
		}
	}
	return out
}

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

// SetTargetBias stores doctrine-derived target scoring multipliers.
func (e *Engine) SetTargetBias(b TargetBias) {
	e.mu.Lock()
	e.bias = b
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

// logProductionDiagnostics logs production queue state every 100 ticks,
// regardless of whether rules fired. Helps debug "why isn't X being produced?"
// when other rules (e.g. infantry) are still firing normally.
var lastProdDiagTick int

func logProductionDiagnostics(env RuleEnv) {
	gs := env.State
	if gs.Tick-lastProdDiagTick < 100 {
		return
	}
	lastProdDiagTick = gs.Tick

	for _, pq := range gs.ProductionQueues {
		slog.Info("production diagnostics",
			"queue", pq.Type,
			"busy", pq.CurrentItem != "" && pq.CurrentProgress < 100,
			"currentItem", pq.CurrentItem,
			"buildable", strings.Join(pq.Buildable, ","),
		)
	}
	slog.Info("production diagnostics",
		"cash", gs.Player.Cash,
		"powerState", gs.Player.PowerState,
		"powerExcess", gs.Player.PowerProvided-gs.Player.PowerDrained,
	)
}

// logProduceInfantryGate reports every N ticks WHICH gates of the
// produce-infantry rule are currently blocking it from firing. Used to
// answer "why is production throughput only 17% of theoretical?" — the
// rule fire_count only shows successful fires, not why the rule didn't
// fire. This dumps the state of each gate so we can see what's actually
// blocking (cash / queue busy / unit cap / axis burned).
var lastInfantryGateDiagTick int

func logProduceInfantryGate(env RuleEnv) {
	gs := env.State
	if gs.Tick-lastInfantryGateDiagTick < 500 {
		return
	}
	lastInfantryGateDiagTick = gs.Tick

	// Evaluate each gate of the produce-infantry rule independently.
	axisBurned := env.AxisBurned("infantry")
	hasBarracks := env.HasRole("barracks")
	queueBusy := env.QueueBusy("Infantry")
	canBuildE1 := env.CanBuild("Infantry", "e1")
	e1Count := env.UnitCount("e1")

	// Also check the rush-variant gate.
	isRushed := env.IsRushed()

	// Total non-cash gate result: would the rule pass all non-cash gates?
	gatesPassExceptCash := !axisBurned && hasBarracks && !queueBusy && canBuildE1
	// Rush-variant: passes if rushed + others.
	rushGatesPassExceptCash := isRushed && !axisBurned && hasBarracks && !queueBusy && canBuildE1

	// Cash — we don't know the doctrine cap or exact reservations from here,
	// but the base cash cost for a rifle is 100 (normal) or 50 (rush).
	cash := gs.Player.Cash

	slog.Info("produce-infantry gate diagnostic",
		"tick", gs.Tick,
		"cash", cash,
		"axisBurned", axisBurned,
		"hasBarracks", hasBarracks,
		"queueBusy", queueBusy,
		"canBuildE1", canBuildE1,
		"e1Count", e1Count,
		"isRushed", isRushed,
		"gatesPassExceptCash", gatesPassExceptCash,
		"rushGatesPassExceptCash", rushGatesPassExceptCash,
	)
}

// logCashFlow tracks account balance (Cash + Resources) over time and
// reports net delta per 500-tick window. Can't decompose income vs
// spending without wiring up per-command cash tracking, but the net
// delta tells us whether the economy is net-positive or net-negative,
// and the pattern of spending vs harvester_return firings tells us
// roughly whether income is the bottleneck (few returns) or spending is
// (returns coming but cash still zero).
var (
	lastCashFlowTick      int
	lastCashSnapshot      int
	lastResourcesSnapshot int
)

func logCashFlow(env RuleEnv) {
	gs := env.State
	if gs.Tick-lastCashFlowTick < 500 {
		return
	}
	cashDelta := gs.Player.Cash - lastCashSnapshot
	resDelta := gs.Player.Resources - lastResourcesSnapshot
	totalAvailable := gs.Player.Cash + gs.Player.Resources
	totalDelta := cashDelta + resDelta
	slog.Info("cash flow diagnostic",
		"tick", gs.Tick,
		"cash", gs.Player.Cash,
		"resources", gs.Player.Resources,
		"total_available", totalAvailable,
		"cash_delta_500t", cashDelta,
		"resources_delta_500t", resDelta,
		"total_delta_500t", totalDelta,
		"power_state", gs.Player.PowerState,
	)
	lastCashFlowTick = gs.Tick
	lastCashSnapshot = gs.Player.Cash
	lastResourcesSnapshot = gs.Player.Resources
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
