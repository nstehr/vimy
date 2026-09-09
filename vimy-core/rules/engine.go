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
	Source       string
}

// Engine runs compiled rules against game state each tick, in priority order.
// An exclusive rule blocks lower-priority rules in its category, so two rules
// can't issue conflicting orders on the same queue.
type Engine struct {
	mu      sync.RWMutex
	rules   []*Rule
	Memory  map[string]any
	memMu   sync.Mutex // guards all reads/writes to Memory
	Terrain *model.TerrainGrid
	prefs   UnitPreferences
	bias    TargetBias

	exporter *StateExporter

	// Per-rule firing counters for the current doctrine window, populated only
	// while traceFirings is on and reset by FlushFiringStats.
	statsMu      sync.Mutex
	traceFirings bool
	fireCounts   map[string]int
	actCounts    map[string]int
	firstTick    map[string]int
	lastTick     map[string]int
}

// RuleFiringStats captures how often a rule fired during a doctrine window
// and the tick range over which it fired.
type RuleFiringStats struct {
	FireCount int
	// ActCount is the firings that sent an order or moved something in memory.
	// FireCount only counts matching conditions — a rule can match every tick
	// and never act — so the two are only meaningful read together.
	ActCount  int
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
		actCounts:  make(map[string]int),
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

// SetTraceFirings toggles per-rule firing instrumentation, off by default.
// Takes effect on subsequent Evaluate calls.
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

// recordFiring is a no-op when tracing is off — one early return per firing.
func (e *Engine) recordFiring(name string, tick int, acted bool) {
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
	if acted {
		e.actCounts[name]++
	}
}

// FlushFiringStats atomically reads and clears the window's counters, so the
// caller can attach them to the doctrine record that just ended.
func (e *Engine) FlushFiringStats() map[string]RuleFiringStats {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	out := make(map[string]RuleFiringStats, len(e.fireCounts))
	for name, count := range e.fireCounts {
		out[name] = RuleFiringStats{
			FireCount: count,
			ActCount:  e.actCounts[name],
			FirstTick: e.firstTick[name],
			LastTick:  e.lastTick[name],
		}
	}
	e.fireCounts = make(map[string]int)
	e.actCounts = make(map[string]int)
	e.firstTick = make(map[string]int)
	e.lastTick = make(map[string]int)
	return out
}

// FiringStatsSnapshot copies the window's counters without resetting them.
func (e *Engine) FiringStatsSnapshot() map[string]RuleFiringStats {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	out := make(map[string]RuleFiringStats, len(e.fireCounts))
	for name, count := range e.fireCounts {
		out[name] = RuleFiringStats{
			FireCount: count,
			ActCount:  e.actCounts[name],
			FirstTick: e.firstTick[name],
			LastTick:  e.lastTick[name],
		}
	}
	return out
}

// RuleNames is the doctrine window's available rule set. Paired with
// FlushFiringStats it yields the rules that could have fired and didn't.
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
	sampleIncome(env)
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

		// A matching condition is not an action that did something: an action
		// can return early on preconditions the rule doesn't mirror, and
		// counting those made the busiest rules in the archive the idlest ones.
		//
		// Orders on the wire are the signal; markEffect covers the two actions
		// whose work never leaves memory.
		var sentBefore uint64
		if conn != nil {
			sentBefore = conn.Sent()
		}
		delete(env.Memory, effectKey)

		if err := r.Action(env, conn); err != nil {
			slog.Error("rule action error", "rule", r.Name, "error", err)
		}

		acted, _ := env.Memory[effectKey].(bool)
		if conn != nil && conn.Sent() > sentBefore {
			acted = true
		}
		delete(env.Memory, effectKey)
		e.recordFiring(r.Name, gs.Tick, acted)
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

// Swap atomically replaces the rule set. Compiles first, so a doctrine that
// fails to compile leaves the running one in place.
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

	// Keep squads the incoming rule set still forms. Dropping all of them on
	// every swap meant no squad survived long enough to reach commit strength,
	// which read in the logs as combat attrition.
	//
	// Orphans still have to go: nothing would reinforce or command them, and
	// their members would stay assigned forever, invisible to the idle pool.
	kept, dropped := e.retainSquads(squadNames(compiled))
	slog.Info("rule set swapped", "count", len(compiled), "kept_squads", kept,
		"dropped_squads", dropped, "rules", names)
	return nil
}

// squadNames reads squad names off ActionSrc, the source text vimyc emitted —
// a compiled closure can't be asked what arguments it captured.
func squadNames(rules []*Rule) map[string]bool {
	out := make(map[string]bool)
	for _, r := range rules {
		if r.ActionSrc == "" {
			continue
		}
		name, args, err := parseActionSrc(r.ActionSrc)
		if err != nil || name != "form-squad" || len(args) == 0 {
			continue
		}
		out[args[0]] = true
	}
	return out
}

// retainSquads drops every squad not in `keep`, and reports both counts.
func (e *Engine) retainSquads(keep map[string]bool) (kept, dropped int) {
	e.memMu.Lock()
	defer e.memMu.Unlock()
	squads := getSquads(e.Memory)
	for name := range squads {
		if keep[name] {
			kept++
			continue
		}
		delete(squads, name)
		delete(e.Memory, "huntBase:"+name)
		dropped++
	}
	e.Memory["squads"] = squads
	return kept, dropped
}

// Reset wipes per-game memory and firing counters, keeping compiled rules and
// terrain.
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

// LockMemory lets the strategist read Memory from its own goroutine. Callers
// must pair it with UnlockMemory.
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
			Source:       r.Source,
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

// logIdleDiagnostics answers "why isn't the AI doing anything?" — dumps queue
// state when no rule fires, throttled.
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

// logMilitaryDiagnostics answers "why doesn't the AI attack?" — runs on a tick
// interval regardless of rule activity.
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

// logProductionDiagnostics answers "why isn't X being produced?" when unrelated
// rules are still firing normally.
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

// logProduceInfantryGate reports which of produce-infantry's gates are blocking
// it. fire_count records successful fires only, and says nothing about why a
// rule stayed silent.
var lastInfantryGateDiagTick int

func logProduceInfantryGate(env RuleEnv) {
	gs := env.State
	if gs.Tick-lastInfantryGateDiagTick < 500 {
		return
	}
	lastInfantryGateDiagTick = gs.Tick

	axisBurned := env.AxisBurned("infantry")
	hasBarracks := env.HasRole("barracks")
	queueBusy := env.QueueBusy("Infantry")
	canBuildE1 := env.CanBuild("Infantry", "e1")
	e1Count := env.UnitCount("e1")

	isRushed := env.IsRushed()

	gatesPassExceptCash := !axisBurned && hasBarracks && !queueBusy && canBuildE1
	rushGatesPassExceptCash := isRushed && !axisBurned && hasBarracks && !queueBusy && canBuildE1

	// Base rifle cost; the doctrine cap and reservations aren't visible here.
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

// logCashFlow reports net change in Cash + Resources per window. Income and
// spending can't be separated without per-command cash tracking, but the sign
// of the delta against harvester_return firings distinguishes an income
// bottleneck (few returns) from a spending one (returns arriving, cash still
// zero).
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
	// Stable, so ties resolve identically every run. Two rules whose priorities
	// lerp on different doctrine knobs will collide for some doctrine, and
	// resolving that properly would mean ranking the knobs against each other.
	// Source order decides instead, and vimyc emits in a fixed order, so the
	// resolution is at least inspectable.
	sort.SliceStable(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})
	return rules, nil
}

// incomeSampleTicks — wide enough that one purchase isn't a collapse in income,
// narrow enough that a rule reacting to it is reacting to now.
const incomeSampleTicks = 500

// sampleIncome keeps the net cash delta in engine memory: a rate needs two
// observations and a RuleEnv sees one.
func sampleIncome(env RuleEnv) {
	// The same total the rules spend from. Player.Cash is the spendable half;
	// ore waiting in the silos sits in Player.Resources, and RuleEnv.Cash sums
	// both. Sampling the spendable half alone gave a delta that hovered at zero
	// and was never positive in any state of game 89 — so `income-rate > 0`,
	// the release the savings model depends on, could never fire. The model was
	// permanently armed and removed 36 of the 43 percentage points of states
	// where a base defence was affordable.
	cash := env.State.Player.Cash + env.State.Player.Resources
	tick := env.State.Tick
	last, haveLast := env.Memory["incomeSampleTick"].(int)
	prev, havePrev := env.Memory["incomeSampleCash"].(int)
	if !haveLast || !havePrev {
		env.Memory["incomeSampleTick"] = tick
		env.Memory["incomeSampleCash"] = cash
		return
	}
	if tick-last < incomeSampleTicks {
		return
	}
	env.Memory["incomeRate"] = cash - prev
	env.Memory["incomeSampleTick"] = tick
	env.Memory["incomeSampleCash"] = cash
}
