package agent

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"

	baml_client "github.com/nstehr/vimy/vimy-core/baml_client"
	"github.com/nstehr/vimy/vimy-core/baml_client/types"
	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
)

// GameResult represents the outcome of a single game.
type GameResult struct {
	Player  string // our player name
	Faction string
	Won     bool
}

// DoctrineRecord is a timestamped doctrine output from the LLM.
type DoctrineRecord struct {
	Tick          int
	Doctrine      rules.Doctrine
	Events        []Event
	HasEnemyIntel bool

	// Rule-engine trace for the window this doctrine was active.
	// RuleSet is the list of rules compiled in at doctrine swap time (the
	// "available" set). RuleStats is filled in once the window closes — on
	// the next doctrine swap or at game end.
	RuleSet   []string
	RuleStats map[string]rules.RuleFiringStats
}

// TypeCount is a type name with a count, used for display purposes.
type TypeCount struct {
	Type  string
	Count int
}

// BattlefieldStatus summarises our losses and enemy composition for dashboard display.
type BattlefieldStatus struct {
	InfantryLost       int
	VehiclesLost       int
	AircraftLost       int
	NavalLost          int
	EnemyBuildings     []TypeCount // currently visible
	EnemyBuildingsSeen []TypeCount // cumulative historical
	EnemyUnits         []TypeCount // currently visible
	EnemyUnitsSeen     []TypeCount // cumulative historical
}

// Strategist runs in the background, periodically consulting the LLM
// to generate a doctrine and swap the rule engine's rule set.
type Strategist struct {
	mu              sync.Mutex
	latest          *model.GameState
	engine          *rules.Engine
	faction         string
	opponentFaction string // set from HelloMessage.Opponents; "unknown" if absent
	directive       string // initial doctrine seed from --doctrine flag
	interval        int    // re-evaluate every N ticks
	lastTick        int    // tick of last evaluation
	ready           chan struct{}
	prevSnap        *stateSnapshot // previous state snapshot for event diff
	// prevCashSnapshot: cash observed at the last strategist evaluation.
	// Used to compute cash_burn_rate (net cash change per interval) surfaced
	// to the LLM as a "is this build order sustainable?" signal.
	prevCashSnapshot int
	prevCashTick     int
	// Compiles doctrines. Required — there is no other compiler.
	compiler *rules.VimycCompiler
	cooldown int              // minimum ticks between event-driven evaluations
	pending  []Event          // events accumulated since last evaluation
	history  []DoctrineRecord // append-only log of all doctrine outputs

	// stressEvents holds high-impact events (harvester/critical-building/
	// strategy-countered) for several evaluation cycles after they fire.
	// Without this, a forced pivot at eval N "consumes" the events feed and
	// eval N+1 sees an empty event list — which the LLM reads as
	// "situation resolved" and reverts the pivot prematurely (vimy-8vc).
	// Keeping these visible across the stressEventTTL window lets the
	// STICKY PIVOT prompt rule actually evaluate the right context.
	stressEvents []Event

	// burnStress holds the same kinds of events but never prunes within a
	// match. Burn detection (computeBurnedAxes) walks this so a counter
	// event remains correlatable with later doctrines hours of play later —
	// without it, game 31 only ever held 1 vehicle counter in the 1500-tick
	// window and never crossed the burn threshold despite 71 vehicle-heavy
	// doctrines (vimy follow-up after game 31).
	burnStress []Event

	// Cumulative loss tracking — independent of event windowing.
	// prevFreshIDs holds per-domain unit IDs from the PREVIOUS tick (never merged).
	// totalLosses accumulates deaths across the entire game.
	prevFreshIDs map[string]map[int]bool
	totalLosses  map[string]int

	// Win/loss record — persists across resets within a session.
	record []GameResult

	// Cross-game persistence. Nil-safe: archival is skipped when store is nil.
	store *store.Store

	// Memory retrieval is cached for the duration of a game — one SQLite
	// query on the first evaluate(), reused for subsequent re-evaluations.
	// Cleared on Reset.
	memoryCache     *types.MemoryContext
	memoryAttempted bool
	librarian       *LibrarianSnapshot // last librarian output, for dashboard inspection
}

// SetStore wires in persistent storage for post-game archival and memory
// retrieval. Safe to call nil to disable.
func (s *Strategist) SetStore(st *store.Store) {
	s.mu.Lock()
	s.store = st
	s.mu.Unlock()
}

// NewStrategist creates a strategist. If directive is empty, defaults to "balanced".
func NewStrategist(engine *rules.Engine, directive string, interval int) *Strategist {
	if directive == "" {
		directive = "balanced"
	}
	if interval <= 0 {
		interval = 500
	}
	return &Strategist{
		engine:    engine,
		directive: directive,
		interval:  interval,
		cooldown:  100,
		ready:     make(chan struct{}, 1),
	}
}

// Reset clears all accumulated state so the strategist is ready for a new game.
// The engine reference and directive are preserved.
func (s *Strategist) Reset() {
	s.mu.Lock()
	s.latest = nil
	s.prevSnap = nil
	s.prevCashSnapshot = 0
	s.prevCashTick = 0
	s.pending = nil
	s.stressEvents = nil
	s.burnStress = nil
	s.history = nil
	s.prevFreshIDs = nil
	s.totalLosses = nil
	s.lastTick = 0
	s.memoryCache = nil
	s.memoryAttempted = false
	s.librarian = nil
	s.opponentFaction = ""
	s.mu.Unlock()
	slog.Info("strategist reset")
}

// RecordGame appends a game result to the session record.
func (s *Strategist) RecordGame(result GameResult) {
	s.mu.Lock()
	s.record = append(s.record, result)
	s.mu.Unlock()
	outcome := "LOSS"
	if result.Won {
		outcome = "WIN"
	}
	slog.Info("game recorded", "outcome", outcome, "player", result.Player, "faction", result.Faction)
}

// GetRecord returns a copy of the session win/loss record.
func (s *Strategist) GetRecord() []GameResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]GameResult, len(s.record))
	copy(out, s.record)
	return out
}

// SetFaction sets the faction string (called from HandleHello).
func (s *Strategist) SetFaction(f string) {
	s.mu.Lock()
	s.faction = f
	s.mu.Unlock()
}

// SetOpponents records opponent factions for memory retrieval. v1 picks the
// first non-allied opponent's faction; multi-opponent games degrade gracefully
// to that first entry. An empty list leaves opponentFaction as "unknown".
func (s *Strategist) SetOpponents(opponents []ipc.OpponentInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(opponents) == 0 {
		s.opponentFaction = "unknown"
		return
	}
	s.opponentFaction = opponents[0].Faction
}

// GetOpponentFaction returns the recorded opponent faction, or "unknown" if
// none was set.
func (s *Strategist) GetOpponentFaction() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.opponentFaction == "" {
		return "unknown"
	}
	return s.opponentFaction
}

// GetDirective returns the current directive string.
func (s *Strategist) GetDirective() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.directive
}

// SetDirective updates the directive and signals an immediate re-evaluation.
func (s *Strategist) SetDirective(d string) {
	s.mu.Lock()
	s.directive = d
	s.mu.Unlock()
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// GetCurrentDoctrine returns the most recent doctrine output, or nil if none yet.
func (s *Strategist) GetCurrentDoctrine() *DoctrineRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) == 0 {
		return nil
	}
	rec := s.history[len(s.history)-1]
	return &rec
}

// GetHistory returns a copy of the full doctrine history.
func (s *Strategist) GetHistory() []DoctrineRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DoctrineRecord, len(s.history))
	copy(out, s.history)
	return out
}

// GetRules returns the current compiled rules from the engine.
func (s *Strategist) GetRules() []rules.RuleSummary {
	return s.engine.Rules()
}

// RuleTraceSnapshot is the dashboard-facing view of rule firing
// instrumentation for the current doctrine window.
type RuleTraceSnapshot struct {
	Enabled bool
	RuleSet []string
	Stats   map[string]rules.RuleFiringStats
}

// GetRuleTraceSnapshot returns the live firing counters (non-destructive)
// plus the current rule set. When tracing is disabled, Enabled is false and
// the caller should render the "tracing off" state.
func (s *Strategist) GetRuleTraceSnapshot() RuleTraceSnapshot {
	return RuleTraceSnapshot{
		Enabled: s.engine.TracingEnabled(),
		RuleSet: s.engine.RuleNames(),
		Stats:   s.engine.FiringStatsSnapshot(),
	}
}

// GetBattlefieldStatus returns current losses and enemy composition.
func (s *Strategist) GetBattlefieldStatus() *BattlefieldStatus {
	s.mu.Lock()
	losses := make(map[string]int, len(s.totalLosses))
	for k, v := range s.totalLosses {
		losses[k] = v
	}
	gs := s.latest
	s.mu.Unlock()

	if gs == nil {
		return nil
	}

	status := &BattlefieldStatus{
		InfantryLost: losses["infantry"],
		VehiclesLost: losses["vehicle"],
		AircraftLost: losses["aircraft"],
		NavalLost:    losses["naval"],
	}

	// Current enemy composition from game state.
	enemyUnitCounts := make(map[string]int)
	enemyBuildingCounts := make(map[string]int)
	for _, e := range gs.Enemies {
		if rules.IsKnownBuildingType(e.Type) {
			enemyBuildingCounts[e.Type]++
		} else {
			enemyUnitCounts[e.Type]++
		}
	}
	for t, c := range enemyUnitCounts {
		status.EnemyUnits = append(status.EnemyUnits, TypeCount{Type: t, Count: c})
	}
	for t, c := range enemyBuildingCounts {
		status.EnemyBuildings = append(status.EnemyBuildings, TypeCount{Type: t, Count: c})
	}

	// Historical sightings from engine memory.
	s.engine.LockMemory()
	for t, c := range rules.GetEnemyUnitsSeen(s.engine.Memory) {
		status.EnemyUnitsSeen = append(status.EnemyUnitsSeen, TypeCount{Type: t, Count: c})
	}
	for t, c := range rules.GetEnemyBuildingsSeen(s.engine.Memory) {
		status.EnemyBuildingsSeen = append(status.EnemyBuildingsSeen, TypeCount{Type: t, Count: c})
	}
	s.engine.UnlockMemory()

	return status
}

// UpdateState stores the latest game state, detects events, and signals
// readiness on the first call, interval boundaries, or significant events.
func (s *Strategist) UpdateState(gs model.GameState) {
	s.mu.Lock()
	first := s.latest == nil
	s.latest = &gs

	// --- Cumulative loss tracking (independent of event windowing) ---
	curFresh := map[string]map[int]bool{
		"infantry": make(map[int]bool),
		"vehicle":  make(map[int]bool),
		"aircraft": make(map[int]bool),
		"naval":    make(map[int]bool),
	}
	for _, u := range gs.Units {
		d := unitDomain(u)
		if d != "" {
			curFresh[d][u.ID] = true
		}
	}
	if s.prevFreshIDs != nil {
		if s.totalLosses == nil {
			s.totalLosses = make(map[string]int)
		}
		for domain, prevIDs := range s.prevFreshIDs {
			for id := range prevIDs {
				if !curFresh[domain][id] {
					s.totalLosses[domain]++
				}
			}
		}
	}
	s.prevFreshIDs = curFresh

	// Detect events against the previous snapshot.
	// prevSnap's domain ID sets are accumulated (high-water mark) so that
	// losses add up across multiple state updates instead of resetting each tick.
	s.engine.LockMemory()
	events := detectEvents(gs, s.engine.Memory, s.prevSnap)
	snap := takeSnapshot(gs, s.engine.Memory)
	s.engine.UnlockMemory()

	if s.prevSnap != nil {
		snap.lastCounterTick = s.prevSnap.lastCounterTick
		snap.lastHarvesterAttackTick = s.prevSnap.lastHarvesterAttackTick

		counterFired := false
		for _, e := range events {
			if e.Kind == EventStrategyCountered {
				snap.lastCounterTick = gs.Tick
				counterFired = true
			}
			if e.Kind == EventHarvesterUnderAttack {
				snap.lastHarvesterAttackTick = gs.Tick
			}
		}

		if counterFired {
			// Counter event fired: reset baseline to fresh snapshot.
			snap.lossBaselineTick = gs.Tick
		} else if gs.Tick-s.prevSnap.lossBaselineTick < counterCooldownTicks {
			// Within accumulation window: carry forward union of unit IDs
			// so losses accumulate across multiple game state updates.
			snap.infantryIDs = mergeIDSets(s.prevSnap.infantryIDs, snap.infantryIDs)
			snap.vehicleIDs = mergeIDSets(s.prevSnap.vehicleIDs, snap.vehicleIDs)
			snap.aircraftIDs = mergeIDSets(s.prevSnap.aircraftIDs, snap.aircraftIDs)
			snap.lossBaselineTick = s.prevSnap.lossBaselineTick
		} else {
			// Accumulation window expired: reset baseline.
			snap.lossBaselineTick = gs.Tick
		}
	} else {
		snap.lossBaselineTick = gs.Tick
	}

	s.prevSnap = &snap
	s.pending = append(s.pending, events...)

	shouldSignal := first || (gs.Tick-s.lastTick >= s.interval)
	if !shouldSignal && len(events) > 0 && (gs.Tick-s.lastTick >= s.cooldown) {
		shouldSignal = true
	}
	s.mu.Unlock()

	if shouldSignal {
		select {
		case s.ready <- struct{}{}:
		default:
		}
	}
}

// Start launches the background strategist goroutine. It blocks until ctx is cancelled.
func (s *Strategist) Start(ctx context.Context) {
	slog.Info("strategist started", "directive", s.directive, "interval", s.interval)
	for {
		select {
		case <-ctx.Done():
			slog.Info("strategist stopped")
			return
		case <-s.ready:
			s.evaluate(ctx)
		}
	}
}

func (s *Strategist) evaluate(ctx context.Context) {
	s.mu.Lock()
	gs := s.latest
	faction := s.faction
	events := s.pending
	s.pending = nil
	losses := make(map[string]int, len(s.totalLosses))
	for k, v := range s.totalLosses {
		losses[k] = v
	}
	// Widened from 8 to 16 (vimy-b9a) so the BURNED AXIS prompt rule can
	// see repeated pivots that are 10-20 doctrines apart. Game 21 had 4
	// air pivots across the match each followed by aircraft losses, but
	// the LLM never saw 2 in a single window.
	recentDoctrines := recentDoctrineSummaries(s.history, 16)

	// Persist high-impact events so the LLM continues to see the stress
	// signal across several evaluations after the original event fired
	// (vimy-8vc). Without this, a forced pivot consumes the event feed and
	// the next eval reads it as "resolved", reverting the pivot.
	currentTick := 0
	if gs != nil {
		currentTick = gs.Tick
	}
	s.stressEvents = pruneStressEvents(s.stressEvents, currentTick)
	s.stressEvents = appendStressEvents(s.stressEvents, events)
	// burnStress accumulates the same kinds of events for the full match.
	// Burn detection needs cross-match correlation between pivots and counter
	// events, not just a 1500-tick sliding window.
	s.burnStress = appendStressEvents(s.burnStress, events)
	stressSnapshot := append([]Event(nil), s.stressEvents...)
	burnedAxes := computeBurnedAxes(s.history, s.burnStress, currentTick)
	beingRushed, harvesterHarassed := computePressureFlags(currentTick, gs, s.stressEvents)
	s.mu.Unlock()

	if gs == nil {
		return
	}

	for _, e := range events {
		slog.Info("event detected", "kind", e.Kind, "tick", e.Tick, "detail", e.Detail)
	}
	slog.Debug("strategist evaluating", "tick", gs.Tick, "directive", s.directive, "events", len(events))

	s.engine.LockMemory()
	swFires := snapshotSuperweaponFires(s.engine.Memory)
	mergedEvents := mergeStressEvents(events, stressSnapshot)
	situation := buildSituation(*gs, s.engine.Memory, mergedEvents, swFires, losses)
	situation.Recent_doctrines = recentDoctrines
	situation.Burned_axes = burnedAxes
	situation.Being_rushed = beingRushed
	situation.Harvester_harassed = harvesterHarassed
	// Situation signals paired with the new tempo knobs (commit_ratio etc.).
	// Ready ratio comes from squad state in engine memory; cash burn rate
	// from the delta since our last evaluation; enemy-reach estimate from
	// distance to any known enemy base.
	situation.Ground_squad_ready_ratio = groundSquadReadyRatio(s.engine.Memory, *gs)
	situation.Cash_burn_rate = int64(s.computeCashBurnRate(gs))
	// Named rather than inferred: the roster lists in the prompt are keyed by
	// side, and asking the model to know that germany is Allied does not work.
	situation.Faction_side = rules.SideOf(faction)
	situation.Unbuildable_roles = rules.UnbuildableRoles(faction)
	situation.Time_to_reach_enemy_estimate = int64(timeToReachEnemyEstimate(s.engine.Memory, *gs))
	// Push burned axes into engine memory so production rules can hard-gate
	// on AxisBurned() — previously the constraint was prompt-only and the
	// LLM kept committing aircraft despite repeated Flak counters (game 27).
	burnedSet := make(map[string]bool, len(burnedAxes))
	for _, a := range burnedAxes {
		burnedSet[a] = true
	}
	s.engine.Memory["burnedAxes"] = burnedSet
	// Push pressure flags into engine memory so production/defense rules can
	// read them (mirrors burnedAxes). Lets the rule layer respond to a rush
	// with cheaper cash gates and higher defense priority — the prompt-side
	// rush response sets the doctrine shape but can't move the cash floors
	// or rule priorities by itself.
	s.engine.Memory["beingRushed"] = beingRushed
	s.engine.Memory["harvesterHarassed"] = harvesterHarassed
	// Doctrine-level repair budget cap (0 = disabled, current behavior).
	if len(s.history) > 0 {
		latest := s.history[len(s.history)-1].Doctrine
		s.engine.Memory["repairBudgetRatio"] = latest.RepairBudgetRatio
		s.engine.Memory["scoutReachPriority"] = latest.ScoutReachPriority
	}
	enemyBases, _ := s.engine.Memory["enemyBases"].(map[string]rules.EnemyBaseIntel)
	hasEnemyIntel := len(enemyBases) > 0
	s.engine.UnlockMemory()

	memoryCtx := s.ensureMemory(ctx, gs.MapWidth, gs.MapHeight)
	bamlDoctrine, err := baml_client.GenerateDoctrine(ctx, s.directive, situation, faction, memoryCtx)
	if err != nil {
		slog.Error("strategist LLM call failed", "error", err)
		return
	}

	doctrine := fromBAML(bamlDoctrine)
	doctrine.Validate()

	// The strategist is told which roles are faction-locked and told to list
	// only its own. It does so most of the time. When it does not, the unit is
	// skipped where one is chosen — but `DoctrineParams` reads the raw list, so
	// naming the other side's unit still moves rule priorities on the strength
	// of a name. Dropped here, before anything reads it.
	var dropped []string
	doctrine, dropped = rules.FilterPreferences(doctrine, faction)
	if len(dropped) > 0 {
		slog.Warn("doctrine named units this faction cannot build",
			"faction", faction, "doctrine", doctrine.Name, "dropped", dropped)
	}

	// Close out the previous doctrine window: flush the engine's firing
	// counters and attach them to the last DoctrineRecord. The stats
	// reflect rule activity during the window that just ended. No-op
	// when rule tracing is disabled.
	tracingOn := s.engine.TracingEnabled()
	var priorStats map[string]rules.RuleFiringStats
	if tracingOn {
		priorStats = s.engine.FlushFiringStats()
	}

	s.mu.Lock()
	if n := len(s.history); n > 0 && len(priorStats) > 0 {
		s.history[n-1].RuleStats = priorStats
	}
	s.history = append(s.history, DoctrineRecord{
		Tick:          gs.Tick,
		Doctrine:      doctrine,
		Events:        events,
		HasEnemyIntel: hasEnemyIntel,
	})
	s.mu.Unlock()

	slog.Info("doctrine generated",
		"name", doctrine.Name,
		"rationale", doctrine.Rationale,
		"economy", doctrine.EconomyPriority,
		"aggression", doctrine.Aggression,
		"groundDefense", doctrine.GroundDefensePriority,
		"airDefense", doctrine.AirDefensePriority,
		"infantry", doctrine.InfantryWeight,
		"vehicle", doctrine.VehicleWeight,
		"air", doctrine.AirWeight,
		"naval", doctrine.NavalWeight,
		"groundAttackGroup", doctrine.GroundAttackGroupSize,
		"airAttackGroup", doctrine.AirAttackGroupSize,
		"navalAttackGroup", doctrine.NavalAttackGroupSize,
		"specialistInfantry", doctrine.SpecializedInfantryWeight,
		"superweapon", doctrine.SuperweaponPriority,
		"prefInfantry", doctrine.PreferredInfantry,
		"prefVehicle", doctrine.PreferredVehicle,
		"prefAircraft", doctrine.PreferredAircraft,
		"prefNaval", doctrine.PreferredNaval,
		"transportAssault", doctrine.TransportAssault,
	)

	s.engine.SetPreferences(rules.UnitPreferences{
		Infantry: doctrine.PreferredInfantry,
		Vehicle:  doctrine.PreferredVehicle,
		Aircraft: doctrine.PreferredAircraft,
		Naval:    doctrine.PreferredNaval,
	})

	s.engine.SetTargetBias(rules.ComputeTargetBias(doctrine))

	compiled, err := s.compile(doctrine)
	if err != nil {
		// The engine keeps the rule set it has. Falling back to
		// Anything else would be worse than doing nothing: it would hide
		// exactly the failure this path exists to expose.
		slog.Error("doctrine did not compile; keeping the current rules",
			"doctrine", doctrine.Name, "error", err)
		return
	}
	if err := s.engine.Swap(compiled); err != nil {
		slog.Error("strategist rule swap failed", "error", err)
		return
	}

	// Capture the rule set available in the window that just opened so a
	// coding agent can later diff "available" vs "actually fired". Skip
	// when tracing is disabled — the rule_set_json archive is paired with
	// firing stats and only meaningful alongside them.
	s.mu.Lock()
	if tracingOn {
		ruleSet := s.engine.RuleNames()
		if n := len(s.history); n > 0 {
			s.history[n-1].RuleSet = ruleSet
		}
	}
	s.lastTick = gs.Tick
	s.mu.Unlock()
}

// UseVimyc sets the compiler doctrines go through. Required: a strategist
// without one cannot turn a doctrine into anything.
func (s *Strategist) UseVimyc(c *rules.VimycCompiler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compiler = c
}

func (s *Strategist) compile(d rules.Doctrine) ([]*rules.Rule, error) {
	s.mu.Lock()
	c := s.compiler
	s.mu.Unlock()
	if c == nil {
		return nil, fmt.Errorf("no compiler configured")
	}
	return c.Compile(d)
}

// fromBAML converts the BAML-generated Doctrine type to our rules.Doctrine.
func fromBAML(d types.Doctrine) rules.Doctrine {
	return rules.Doctrine{
		Name:                      d.Name,
		Rationale:                 d.Rationale,
		EconomyPriority:           d.Economy_priority,
		Aggression:                d.Aggression,
		GroundDefensePriority:     d.Ground_defense_priority,
		AirDefensePriority:        d.Air_defense_priority,
		TechPriority:              d.Tech_priority,
		InfantryWeight:            d.Infantry_weight,
		VehicleWeight:             d.Vehicle_weight,
		AirWeight:                 d.Air_weight,
		NavalWeight:               d.Naval_weight,
		GroundAttackGroupSize:     int(d.Ground_attack_group_size),
		AirAttackGroupSize:        int(d.Air_attack_group_size),
		NavalAttackGroupSize:      int(d.Naval_attack_group_size),
		ScoutPriority:             d.Scout_priority,
		SpecializedInfantryWeight: d.Specialized_infantry_weight,
		SuperweaponPriority:       d.Superweapon_priority,
		CapturePriority:           d.Capture_priority,
		TransportAssault:          d.Transport_assault,
		PreferredInfantry:         d.Preferred_infantry,
		PreferredVehicle:          d.Preferred_vehicle,
		PreferredAircraft:         d.Preferred_aircraft,
		PreferredNaval:            d.Preferred_naval,
		CommitRatio:               d.Commit_ratio,
		BaseDefenseFloor:          int(d.Base_defense_floor),
		RepairBudgetRatio:         d.Repair_budget_ratio,
		ScoutReachPriority:        d.Scout_reach_priority,
	}
}

// snapshotSuperweaponFires computes superweapon fire deltas and updates
// the memory snapshot for the next evaluation cycle. Returns the fire
// entries for inclusion in the GameSituation struct.
func snapshotSuperweaponFires(memory map[string]any) []types.SuperweaponFire {
	totalFires := rules.GetSuperweaponFires(memory)
	if len(totalFires) == 0 {
		return nil
	}

	lastSeenFires, _ := memory["superweaponFiresSnapshot"].(map[string]int)

	var fires []types.SuperweaponFire
	for key, total := range totalFires {
		recent := total - lastSeenFires[key]
		fires = append(fires, types.SuperweaponFire{
			Key:          key,
			Total_fires:  int64(total),
			Recent_fires: int64(recent),
		})
	}

	// Snapshot for next eval's delta
	snapshot := make(map[string]int, len(totalFires))
	for k, v := range totalFires {
		snapshot[k] = v
	}
	memory["superweaponFiresSnapshot"] = snapshot

	return fires
}

// stressEventTTL is how long a high-impact event stays in the persisted
// feed after its original fire tick. 1500 ticks ~= 3 evaluation intervals at
// the default 500-tick cadence, which is enough for the STICKY PIVOT prompt
// rule to see the signal across the pivot evaluation and at least one
// follow-up before the LLM is allowed to consider reverting.
const stressEventTTL = 1500

// stressKinds enumerates the event kinds that should persist beyond a
// single evaluation. These all signal sustained pressure that a one-shot
// pivot rarely resolves: economy raids, base damage, and explicit counter
// detection. Lower-impact events (first_contact, phase_transition, etc.)
// are intentionally excluded so the persisted feed stays tight.
var stressKinds = map[EventKind]bool{
	EventCriticalBuildingLost: true,
	EventArmyDevastated:       true,
	EventEconomyCrisis:        true,
	EventStrategyCountered:    true,
	EventHarvesterLost:        true,
	EventHarvesterUnderAttack: true,
}

// pruneStressEvents drops persisted events older than stressEventTTL.
func pruneStressEvents(buf []Event, currentTick int) []Event {
	if len(buf) == 0 {
		return buf
	}
	out := buf[:0]
	for _, e := range buf {
		if currentTick-e.Tick <= stressEventTTL {
			out = append(out, e)
		}
	}
	return out
}

// appendStressEvents copies stress-kind entries from fresh events into the
// persisted buffer, deduping by (Kind, Tick).
func appendStressEvents(buf []Event, fresh []Event) []Event {
	for _, e := range fresh {
		if !stressKinds[e.Kind] {
			continue
		}
		dup := false
		for _, b := range buf {
			if b.Kind == e.Kind && b.Tick == e.Tick {
				dup = true
				break
			}
		}
		if !dup {
			buf = append(buf, e)
		}
	}
	return buf
}

// mergeStressEvents combines fresh events with persisted stress events,
// deduping by (Kind, Tick). Used when handing the event list to the LLM
// so it sees both this-tick events and recent unresolved stress.
func mergeStressEvents(fresh, persisted []Event) []Event {
	if len(persisted) == 0 {
		return fresh
	}
	out := append([]Event(nil), fresh...)
	for _, p := range persisted {
		dup := false
		for _, f := range fresh {
			if f.Kind == p.Kind && f.Tick == p.Tick {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
}

// axisDominantThreshold is the weight at or above which a doctrine is
// considered to "pivot to" that axis. Mirrors the convergence-break wording
// in the prompt (axis weight >= ~0.55).
const axisDominantThreshold = 0.55

// burnedAxisMinPivots is how many pivots-with-counter must occur before an
// axis is marked burned for the rest of the match. Per-axis because air
// counters (SAM/Flak clusters) are essentially permanent — one confirmed
// counter is enough to mark the axis. Infantry/vehicle/naval keep the 2-
// counter threshold so we don't burn an axis off a single bad engagement
// (rifles vs. one tesla doesn't mean abandon all infantry).
var burnedAxisMinPivots = map[string]int{
	"air":      1,
	"infantry": 2,
	"vehicle":  2,
	"naval":    2,
}

// burnedAxisLookbackTicks limits how long after a pivot doctrine a counter
// event "counts" against it. Roughly two evaluation intervals — the same
// window stress events persist for under stressEventTTL.
const burnedAxisLookbackTicks = 1500

// burnedAxisRecoveryTicks lets an axis un-burn after a counter-free period.
// 0 = no recovery (burns persist for the rest of the match). Air gets a
// recovery window so an "air superiority" directive can resume committing
// aircraft once SAMs have plausibly been cleared (post-SEAD). Without this,
// one SAM kill in the first 10k ticks would neuter the directive for the
// rest of a 100k+ tick match even after vimy successfully suppressed the AA
// threat. Other axes keep persistent burn because counter-tech (Tesla, Flame
// Tower) tends to keep stacking through the match.
var burnedAxisRecoveryTicks = map[string]int{
	"air": 5000,
}

// computeBurnedAxes inspects the FULL game history (not just the recent
// window) and returns axes that have been pivoted to burnedAxisMinPivots+
// times AND each pivot was followed within burnedAxisLookbackTicks by a
// domain-specific counter event. The returned list is what the BURNED
// AXIS prompt rule treats as a hard constraint for subsequent doctrines.
//
// vimy-b9a: previously this detection lived only in the prompt and only
// looked at the last 8 doctrines, missing repeat air pivots that were
// 10-20 doctrines apart (game 21: 4 air pivots, none caught).
func computeBurnedAxes(history []DoctrineRecord, stress []Event, currentTick int) []string {
	if len(history) == 0 {
		return nil
	}
	axisCounts := map[string]int{}
	axisLatestCounter := map[string]int{}
	for _, rec := range history {
		d := rec.Doctrine
		var axis string
		switch {
		case d.AirWeight >= axisDominantThreshold:
			axis = "air"
		case d.InfantryWeight >= axisDominantThreshold:
			axis = "infantry"
		case d.NavalWeight >= axisDominantThreshold:
			axis = "naval"
		case d.VehicleWeight >= axisDominantThreshold:
			axis = "vehicle"
		default:
			continue
		}
		if t, ok := latestAxisCounter(rec.Tick, axis, stress); ok {
			axisCounts[axis]++
			if t > axisLatestCounter[axis] {
				axisLatestCounter[axis] = t
			}
		}
	}
	var out []string
	for axis, n := range axisCounts {
		threshold, ok := burnedAxisMinPivots[axis]
		if !ok {
			threshold = 2
		}
		if n < threshold {
			continue
		}
		// Recovery window: if a recovery period is configured for this axis
		// and the most recent counter is older than that, the axis un-burns.
		if rt, has := burnedAxisRecoveryTicks[axis]; has && rt > 0 {
			if currentTick-axisLatestCounter[axis] > rt {
				continue
			}
		}
		out = append(out, axis)
	}
	return out
}

// latestAxisCounter returns the tick of the latest stress event that
// matches the given axis and fired within burnedAxisLookbackTicks ticks
// AFTER pivotTick. Returns (0, false) when no matching event exists.
func latestAxisCounter(pivotTick int, axis string, stress []Event) (int, bool) {
	latest := 0
	found := false
	for _, e := range stress {
		if e.Tick < pivotTick || e.Tick-pivotTick > burnedAxisLookbackTicks {
			continue
		}
		if !eventMatchesAxis(e, axis) {
			continue
		}
		if e.Tick > latest {
			latest = e.Tick
		}
		found = true
	}
	return latest, found
}

// eventMatchesAxis is the same matching rule axisCounteredAfter uses,
// factored out so latestAxisCounter and axisCounteredAfter can share it.
func eventMatchesAxis(e Event, axis string) bool {
	detail := strings.ToLower(e.Detail)
	switch axis {
	case "air":
		return strings.Contains(detail, "aircraft") || strings.Contains(detail, "sam") || strings.Contains(detail, "flak")
	case "infantry":
		return strings.Contains(detail, "infantry") || strings.Contains(detail, "flame") || strings.Contains(detail, "tesla")
	case "vehicle":
		return e.Kind == EventArmyDevastated
	case "naval":
		return strings.Contains(detail, "naval") || strings.Contains(detail, "submarine") || strings.Contains(detail, "destroyer")
	}
	return false
}

// axisCounteredAfter reports whether a stress event matching the axis fired
// within burnedAxisLookbackTicks ticks AFTER the given pivot tick. Matching
// uses event detail strings — strategy_countered events carry the domain
// in their detail (e.g. "aircraft taking heavy losses", "infantry taking
// heavy losses", "army_devastated" implies vehicle/ground commitment).
func axisCounteredAfter(pivotTick int, axis string, stress []Event) bool {
	for _, e := range stress {
		if e.Tick < pivotTick || e.Tick-pivotTick > burnedAxisLookbackTicks {
			continue
		}
		detail := strings.ToLower(e.Detail)
		switch axis {
		case "air":
			if strings.Contains(detail, "aircraft") || strings.Contains(detail, "sam") || strings.Contains(detail, "flak") {
				return true
			}
		case "infantry":
			if strings.Contains(detail, "infantry") || strings.Contains(detail, "flame") || strings.Contains(detail, "tesla") {
				return true
			}
		case "vehicle":
			if e.Kind == EventArmyDevastated {
				return true
			}
		case "naval":
			if strings.Contains(detail, "naval") || strings.Contains(detail, "submarine") || strings.Contains(detail, "destroyer") {
				return true
			}
		}
	}
	return false
}

// Pressure-flag thresholds (vimy-w13). The user observation that prompted
// this: in a rush scenario harvesters will ALWAYS be under attack — small
// base, no perimeter. So we can't treat "harvester_under_attack present" as
// a single emergency signal. Split it into two cases that need different
// doctrine responses:
//
//   - being_rushed: early game + small base + recent harvester pressure.
//     Response should be COUNTER-FORCE (rifle/dog spam, pillbox at the
//     threatened side), not turtle. Killing the rushers is what unblocks
//     the economy, not bunkering.
//
//   - harvester_harassed: mid/late game + established base + sustained
//     harvester losses. Response should be DISPERSAL / ESCORTS / static
//     defense near refineries. Can briefly slow the main push.
//
// Both flags are computed from the stress-event buffer (which already
// persists harvester events for ~1500 ticks) plus current building count.
const (
	// rushTickCutoff was 6000 — too tight for slow-paced rushes. Game 33's
	// first harvester attack came at tick 9980 against a still-small base
	// (<=6 buildings), but being_rushed was false because the tick window
	// had already closed, so the soft harvester_harassed rule fired
	// instead of the stronger rush response. Widening to 10500 lets the
	// rush rule catch this tempo.
	rushTickCutoff = 10500
	// Building-count gates removed (was 6, then 12, never fired): even at 12
	// the count was routinely exceeded by tick 9-10k because vimy builds
	// power + refinery + barracks + WF + radar + a few defenses + helipad
	// before raids arrive. The tick window already encodes "early game";
	// adding a building-count condition just made the flag silently false.
	// The rush-mode rules themselves cap their output (cap = 2x infantryCap
	// for rifles, normal defenseCap for pillboxes) so leaving the flag on
	// throughout the rush window can't produce runaway output — it just
	// lets cheap defenders spawn when the doctrine doctrine would normally
	// be cash-gated.
	harassTickFloor          = 10500
	harassMinHarvesterEvents = 2    // sustained pressure, not one-off
	pressureLookbackTicks    = 2000 // "recent" stress events window
)

// computePressureFlags returns (beingRushed, harvesterHarassed) from the
// current tick, game state, and persisted stress events. Both flags can be
// false; at most one should be true at a time given the disjoint tick
// thresholds, but the prompt handles either case independently.
func computePressureFlags(currentTick int, gs *model.GameState, stress []Event) (bool, bool) {
	if gs == nil {
		return false, false
	}

	harvesterAttackEvents := 0
	harvesterLostEvents := 0
	for _, e := range stress {
		if currentTick-e.Tick > pressureLookbackTicks {
			continue
		}
		switch e.Kind {
		case EventHarvesterUnderAttack:
			harvesterAttackEvents++
		case EventHarvesterLost:
			harvesterLostEvents++
		}
	}

	beingRushed := currentTick < rushTickCutoff &&
		(harvesterAttackEvents >= 1 || harvesterLostEvents >= 1)

	harvesterHarassed := currentTick >= harassTickFloor &&
		(harvesterAttackEvents >= harassMinHarvesterEvents || harvesterLostEvents >= 1)

	return beingRushed, harvesterHarassed
}

// recentDoctrineSummaries returns the last n doctrines from history as
// compact shape fingerprints for the strategist prompt. Passing these back to
// groundSquadReadyRatio returns idle/target ratio for the ground-attack squad,
// or 0.0 if the squad hasn't formed yet. Paired with the doctrine's
// commit_ratio knob so the LLM can gauge whether its intended commit threshold
// is achievable this tick.
func groundSquadReadyRatio(memory map[string]any, gs model.GameState) float64 {
	squads, ok := memory["squads"].(map[string]*rules.Squad)
	if !ok {
		return 0
	}
	sq, ok := squads["ground-attack"]
	if !ok || sq.TargetSize <= 0 {
		return 0
	}
	idleSet := make(map[int]bool)
	for _, u := range gs.Units {
		if u.Idle {
			idleSet[u.ID] = true
		}
	}
	idle := 0
	for _, id := range sq.UnitIDs {
		if idleSet[id] {
			idle++
		}
	}
	return float64(idle) / float64(sq.TargetSize)
}

// computeCashBurnRate returns the net cash delta since the last strategist
// evaluation. Positive = income exceeds spending; negative = losing cash.
// First eval returns 0 (no prior snapshot). Stores current cash for next
// eval's diff.
func (s *Strategist) computeCashBurnRate(gs *model.GameState) int {
	if gs == nil {
		return 0
	}
	burn := 0
	if s.prevCashTick > 0 && gs.Tick > s.prevCashTick {
		burn = gs.Player.Cash - s.prevCashSnapshot
	}
	s.prevCashSnapshot = gs.Player.Cash
	s.prevCashTick = gs.Tick
	return burn
}

// timeToReachEnemyEstimate returns a rough tick-count estimate for a ground
// unit to walk from our base centroid to the nearest known enemy base. Uses
// a straight-line distance and a per-tick step size approximation. Returns
// -1 when no enemy base intel exists.
func timeToReachEnemyEstimate(memory map[string]any, gs model.GameState) int {
	bases, ok := memory["enemyBases"].(map[string]rules.EnemyBaseIntel)
	if !ok || len(bases) == 0 {
		return -1
	}
	if len(gs.Buildings) == 0 {
		return -1
	}
	var sumX, sumY int
	for _, b := range gs.Buildings {
		sumX += b.X
		sumY += b.Y
	}
	cx := sumX / len(gs.Buildings)
	cy := sumY / len(gs.Buildings)

	best := math.MaxFloat64
	for _, b := range bases {
		dx := float64(b.X - cx)
		dy := float64(b.Y - cy)
		d := math.Sqrt(dx*dx + dy*dy)
		if d < best {
			best = d
		}
	}
	// Rifle walks ~0.25 cells per tick in RA. Convert map distance to ticks.
	const ticksPerMapUnit = 4.0
	return int(best * ticksPerMapUnit)
}

// recentDoctrineSummaries returns the last n doctrines from history as
// compact shape fingerprints for the strategist prompt. Passing these back to
// the LLM lets it see when its own attempts are converging on a single shape
// that the battlefield keeps rejecting — the signal needed to force a pivot
// that lessons alone don't produce (see vimy-os6).
func recentDoctrineSummaries(history []DoctrineRecord, n int) []types.RecentDoctrine {
	if n <= 0 || len(history) == 0 {
		return nil
	}
	start := len(history) - n
	if start < 0 {
		start = 0
	}
	out := make([]types.RecentDoctrine, 0, len(history)-start)
	for _, rec := range history[start:] {
		d := rec.Doctrine
		shape := fmt.Sprintf(
			"air=%.2f vehicle=%.2f infantry=%.2f ground_def=%.2f air_def=%.2f aggression=%.2f tech=%.2f econ=%.2f",
			d.AirWeight, d.VehicleWeight, d.InfantryWeight,
			d.GroundDefensePriority, d.AirDefensePriority,
			d.Aggression, d.TechPriority, d.EconomyPriority,
		)
		out = append(out, types.RecentDoctrine{
			Tick:  int64(rec.Tick),
			Name:  d.Name,
			Shape: shape,
		})
	}
	return out
}

// buildSituation constructs a structured GameSituation from the current
// game state, memory, events, superweapon fire data, and cumulative losses.
// Pure function (aside from reading memory) — no side effects.
func buildSituation(gs model.GameState, memory map[string]any, events []Event, swFires []types.SuperweaponFire, totalLosses map[string]int) types.GameSituation {
	sit := types.GameSituation{
		Tick:              int64(gs.Tick),
		Phase:             gamePhase(gs),
		Cash:              int64(gs.Player.Cash),
		Resources:         int64(gs.Player.Resources),
		Resource_capacity: int64(gs.Player.ResourceCapacity),
		Power: types.PowerStatus{
			Drained:  int64(gs.Player.PowerDrained),
			Provided: int64(gs.Player.PowerProvided),
			State:    gs.Player.PowerState,
		},
		Enemies_visible:   int64(len(gs.Enemies)),
		Map_width:         int64(gs.MapWidth),
		Map_height:        int64(gs.MapHeight),
		Superweapon_fires: swFires,
	}

	// Enemy units and buildings summary (currently visible)
	enemyUnitCounts := make(map[string]int)
	enemyBuildingCounts := make(map[string]int)
	for _, e := range gs.Enemies {
		if rules.IsKnownBuildingType(e.Type) {
			enemyBuildingCounts[e.Type]++
		} else {
			enemyUnitCounts[e.Type]++
		}
	}
	for t, c := range enemyUnitCounts {
		sit.Enemy_units = append(sit.Enemy_units, types.TypeCount{Type: t, Count: int64(c)})
	}
	for t, c := range enemyBuildingCounts {
		sit.Enemy_buildings = append(sit.Enemy_buildings, types.TypeCount{Type: t, Count: int64(c)})
	}

	// Buildings summary
	buildingCounts := make(map[string]int)
	for _, b := range gs.Buildings {
		buildingCounts[b.Type]++
	}
	for t, c := range buildingCounts {
		sit.Buildings = append(sit.Buildings, types.TypeCount{Type: t, Count: int64(c)})
	}

	// Units summary
	unitCounts := make(map[string]int)
	idleCount := 0
	for _, u := range gs.Units {
		unitCounts[u.Type]++
		if u.Idle {
			idleCount++
		}
	}
	for t, c := range unitCounts {
		sit.Units = append(sit.Units, types.TypeCount{Type: t, Count: int64(c)})
	}
	sit.Idle_unit_count = int64(idleCount)

	// Active production queues
	for _, pq := range gs.ProductionQueues {
		if pq.CurrentItem != "" {
			sit.Active_production = append(sit.Active_production, types.ActiveProduction{
				Queue:    pq.Type,
				Item:     pq.CurrentItem,
				Progress: int64(pq.CurrentProgress),
			})
		}
	}

	// Support powers
	for _, sp := range gs.SupportPowers {
		status := "charging"
		if sp.Ready {
			status = "READY"
		} else if sp.TotalTicks > 0 {
			pct := 100 - (100 * sp.RemainingTicks / sp.TotalTicks)
			status = fmt.Sprintf("%d%%", pct)
		}
		sit.Support_powers = append(sit.Support_powers, types.SupportPowerStatus{
			Key:    sp.Key,
			Status: status,
		})
	}

	// Squads
	if squads := rules.GetSquads(memory); len(squads) > 0 {
		for _, sq := range squads {
			sit.Squads = append(sit.Squads, types.SquadInfo{
				Name:       sq.Name,
				Role:       sq.Role,
				Unit_count: int64(len(sq.UnitIDs)),
			})
		}
	}

	// Historical enemy sightings
	for t, c := range rules.GetEnemyUnitsSeen(memory) {
		sit.Enemy_units_seen = append(sit.Enemy_units_seen, types.TypeCount{Type: t, Count: int64(c)})
	}
	for t, c := range rules.GetEnemyBuildingsSeen(memory) {
		sit.Enemy_buildings_seen = append(sit.Enemy_buildings_seen, types.TypeCount{Type: t, Count: int64(c)})
	}

	// Capturable neutrals (oil derricks, comm centers, hospitals, etc.) —
	// without this the strategist has no signal to raise capture_priority
	// when scouts uncover tech buildings (vimy-b2c).
	if len(gs.Capturables) > 0 {
		capCounts := make(map[string]int)
		for _, c := range gs.Capturables {
			capCounts[c.Type]++
		}
		for t, c := range capCounts {
			sit.Capturables_visible = append(sit.Capturables_visible, types.TypeCount{Type: t, Count: int64(c)})
		}
	}

	// Known enemy bases
	if bases, ok := memory["enemyBases"].(map[string]rules.EnemyBaseIntel); ok {
		for _, base := range bases {
			sit.Known_enemy_bases = append(sit.Known_enemy_bases, types.EnemyBase{
				Owner:          base.Owner,
				X:              int64(base.X),
				Y:              int64(base.Y),
				Last_seen_tick: int64(base.Tick),
			})
		}
	}

	// Recent events
	for _, e := range events {
		sit.Recent_events = append(sit.Recent_events, types.GameEvent{
			Kind:   string(e.Kind),
			Tick:   int64(e.Tick),
			Detail: e.Detail,
		})
	}

	// Cumulative combat stats
	if len(totalLosses) > 0 {
		sit.Combat_stats = &types.CombatStats{
			Infantry_lost: int64(totalLosses["infantry"]),
			Vehicles_lost: int64(totalLosses["vehicle"]),
			Aircraft_lost: int64(totalLosses["aircraft"]),
			Naval_lost:    int64(totalLosses["naval"]),
		}
	}

	return sit
}
