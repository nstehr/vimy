package agent

import (
	"context"
	"log/slog"
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

	// Rule-engine trace for this doctrine's window: RuleSet is what was compiled
	// in at swap time, RuleStats what actually fired, filled in when the window
	// closes.
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
	EnemyUnitsKilled   int         // engine count
	EnemyBuildingsKill int         // engine count
	EnemyBuildings     []TypeCount // currently visible
	EnemyBuildingsSeen []TypeCount // cumulative historical
	EnemyUnits         []TypeCount // currently visible
	EnemyUnitsSeen     []TypeCount // cumulative historical
}

// Strategist consults the LLM in the background for a doctrine, then swaps the
// rule engine's rule set.
type Strategist struct {
	mu              sync.Mutex
	evaluateMu      sync.Mutex
	generation      uint64
	latest          *model.GameState
	engine          *rules.Engine
	faction         string
	opponentFaction string // set from HelloMessage.Opponents; "unknown" if absent
	directive       string // initial doctrine seed from --doctrine flag
	interval        int    // re-evaluate every N ticks
	lastTick        int    // tick of last evaluation
	ready           chan struct{}
	prevSnap        *stateSnapshot // previous state snapshot for event diff
	// Cash at the last evaluation, for the burn rate the LLM reads as "is this
	// build order sustainable?".
	prevCashSnapshot int
	prevCashTick     int
	// Compiles doctrines. Required — there is no other compiler.
	compiler *rules.VimycCompiler
	cooldown int              // minimum ticks between event-driven evaluations
	pending  []Event          // events accumulated since last evaluation
	history  []DoctrineRecord // append-only log of all doctrine outputs

	// High-impact events held for several evaluations after they fire. A pivot
	// otherwise consumes the feed, and the next evaluation reads the empty list
	// as "resolved" and reverts.
	stressEvents []Event

	// The same events, unpruned for the match. Burn detection correlates a
	// counter event with doctrines hours later, which a sliding window loses:
	// one counter per window never crosses the threshold no matter how many
	// doctrines repeat the mistake.
	burnStress []Event

	// Cumulative loss tracking, independent of event windowing: prevFreshIDs is
	// last tick's per-domain IDs, unmerged, and totalLosses runs for the game.
	prevFreshIDs map[string]map[int]bool
	totalLosses  map[string]int

	// Where units were standing when we lost them. See lossTracker.
	losses lossTracker
	// Where harvester time went. See harvesterTracker.
	harvesters harvesterTracker

	// What the two armies are worth. See armyValueTracker.
	army armyValueTracker

	// Win/loss record — persists across resets within a session.
	record []GameResult

	// Cross-game persistence; nil disables archival.
	store *store.Store

	// One memory query per game, on the first evaluate(); cleared on Reset.
	memoryCache     *types.MemoryContext
	memoryAttempted bool
	librarian       *LibrarianSnapshot // last librarian output, for dashboard inspection
}

// SetStore wires in persistent storage; nil disables archival and memory.
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
		// Roughly two minutes of game time. The floor, not the cadence:
		// replanWorthy events bring it forward when something real happens.
		interval = 3000
	}
	return &Strategist{
		engine:    engine,
		directive: directive,
		interval:  interval,
		// An event still has to wait this long. Even a real signal does not
		// warrant re-planning twice inside half a minute — the previous plan
		// has not had time to express itself.
		cooldown: 600,
		ready:    make(chan struct{}, 1),
	}
}

// Reset clears all accumulated state so the strategist is ready for a new game.
// The engine reference and directive are preserved.
func (s *Strategist) Reset() {
	s.mu.Lock()
	s.generation++
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
	s.losses.reset()
	s.army.reset()
	s.harvesters.reset()
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

// SetOpponents records the first non-allied opponent's faction for memory
// retrieval; multi-opponent games degrade to that entry.
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
	s.generation++
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

// GetRuleTraceSnapshot reads the live firing counters without resetting them.
// Enabled is false when tracing is off, and the counters mean nothing.
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

		EnemyUnitsKilled:   gs.Player.UnitsKilled,
		EnemyBuildingsKill: gs.Player.BuildingsKilled,
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
	intel := s.engine.IntelSnapshot()
	for t, c := range intel.UnitsSeen {
		status.EnemyUnitsSeen = append(status.EnemyUnitsSeen, TypeCount{Type: t, Count: c})
	}
	for t, c := range intel.BuildingsSeen {
		status.EnemyBuildingsSeen = append(status.EnemyBuildingsSeen, TypeCount{Type: t, Count: c})
	}

	return status
}

// UpdateState stores the latest game state and detects events, signalling
// readiness on the first call, on interval boundaries, and on significant events.
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
	s.losses.observe(gs)
	s.army.observe(gs)
	s.harvesters.observe(gs)

	// prevSnap's ID sets are a high-water mark, so losses accumulate across state
	// updates rather than resetting every tick.
	s.engine.LockMemory()
	events := detectEvents(gs, s.engine.Memory, s.prevSnap)
	snap := takeSnapshot(gs, s.engine.Memory)
	s.engine.UnlockMemory()

	if s.prevSnap != nil {
		snap.lastCounterTick = s.prevSnap.lastCounterTick
		snap.lastHarvesterAttackTick = s.prevSnap.lastHarvesterAttackTick
		// A snapshot only sees this tick, so without carrying it forward the
		// "remembered" threat lasts exactly one tick and is no memory at all.
		carryThreatMemory(s.prevSnap.threatLastSeen, snap.threatLastSeen, gs.Tick)

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
			snap.lossBaselineTick = gs.Tick
		} else if gs.Tick-s.prevSnap.lossBaselineTick < counterCooldownTicks {
			// Inside the window: union the ID sets so losses accumulate.
			snap.infantryIDs = mergeIDSets(s.prevSnap.infantryIDs, snap.infantryIDs)
			snap.vehicleIDs = mergeIDSets(s.prevSnap.vehicleIDs, snap.vehicleIDs)
			snap.aircraftIDs = mergeIDSets(s.prevSnap.aircraftIDs, snap.aircraftIDs)
			snap.lossBaselineTick = s.prevSnap.lossBaselineTick
		} else {
			snap.lossBaselineTick = gs.Tick
		}
	} else {
		snap.lossBaselineTick = gs.Tick
	}

	s.prevSnap = &snap
	s.pending = append(s.pending, events...)

	// A timer floor, and events that are actually worth re-planning for. Both
	// were far too eager: a 500-tick timer and ANY event on a 100-tick
	// cooldown produced a complete new strategy every ~750 ticks, 30 seconds
	// of game time, 53 times in game 139.
	shouldSignal := first || (gs.Tick-s.lastTick >= s.interval)
	if !shouldSignal && replanWorthy(events) && (gs.Tick-s.lastTick >= s.cooldown) {
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
	slog.Info("strategist started", "directive", s.GetDirective(), "interval", s.interval)
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
	s.evaluateMu.Lock()
	defer s.evaluateMu.Unlock()
	s.mu.Lock()
	generation := s.generation
	directive := s.directive
	gs := s.latest
	faction := s.faction
	events := s.pending
	s.pending = nil
	losses := make(map[string]int, len(s.totalLosses))
	for k, v := range s.totalLosses {
		losses[k] = v
	}
	// Wide enough for the BURNED AXIS prompt rule to see pivots 10-20 doctrines
	// apart — repeated air pivots each followed by losses are otherwise never
	// visible together.
	recentDoctrines := recentDoctrineSummaries(s.history, 16)

	// Keep the stress signal visible for several evaluations; a pivot otherwise
	// consumes the feed and the next one reads the silence as "resolved".
	currentTick := 0
	if gs != nil {
		currentTick = gs.Tick
	}
	s.stressEvents = pruneStressEvents(s.stressEvents, currentTick)
	s.stressEvents = appendStressEvents(s.stressEvents, events)
	// Burn detection correlates pivots with counter events across the whole
	// match, not a sliding window.
	s.burnStress = appendStressEvents(s.burnStress, events)
	stressSnapshot := append([]Event(nil), s.stressEvents...)
	burnedAxes := computeBurnedAxes(s.history, s.burnStress, currentTick)
	beingRushed, harvesterHarassed := computePressureFlags(currentTick, gs, s.stressEvents)
	if gs == nil {
		s.mu.Unlock()
		return
	}

	for _, e := range events {
		slog.Info("event detected", "kind", e.Kind, "tick", e.Tick, "detail", e.Detail)
	}
	slog.Debug("strategist evaluating", "tick", gs.Tick, "directive", directive, "events", len(events))

	s.engine.LockMemory()
	swFires := snapshotSuperweaponFires(s.engine.Memory)
	mergedEvents := mergeStressEvents(events, stressSnapshot)
	situation := buildSituation(*gs, s.engine.Memory, mergedEvents, swFires, losses)
	situation.Recent_doctrines = recentDoctrines
	situation.Burned_axes = burnedAxes
	situation.Being_rushed = beingRushed
	situation.Harvester_harassed = harvesterHarassed
	// Situation signals the tempo knobs are read against.
	situation.Ground_squad_ready_ratio = groundSquadReadyRatio(s.engine.Memory, *gs)
	situation.Cash_burn_rate = int64(s.computeCashBurnRate(gs))
	// Named rather than inferred: the prompt's rosters are keyed by side, and the
	// model does not reliably know that germany is Allied.
	situation.Faction_side = rules.SideOf(faction)
	situation.Unbuildable_roles = rules.UnbuildableRoles(faction)
	situation.Time_to_reach_enemy_estimate = int64(timeToReachEnemyEstimate(s.engine.Memory, *gs))
	enemyBases, _ := s.engine.Memory["enemyBases"].(map[string]rules.EnemyBaseIntel)
	hasEnemyIntel := len(enemyBases) > 0
	s.engine.UnlockMemory()
	s.engine.SetStrategicSignals(rules.StrategicSignals{BurnedAxes: burnedAxes,
		BeingRushed: beingRushed, HarvesterHarassed: harvesterHarassed})
	s.mu.Unlock()

	memoryCtx := s.ensureMemory(ctx, gs.MapWidth, gs.MapHeight, generation)
	bamlDoctrine, err := baml_client.GenerateDoctrine(ctx, directive, situation, faction, memoryCtx)
	if err != nil {
		slog.Error("strategist LLM call failed", "error", err)
		return
	}

	doctrine := fromBAML(bamlDoctrine)
	doctrine.Validate()

	// Dropped before anything reads it: unit selection already skips unbuildable
	// roles, but DoctrineParams reads the raw list and would move rule priorities
	// on the strength of a name.
	var dropped []string
	doctrine, dropped = rules.FilterPreferences(doctrine, faction)
	if len(dropped) > 0 {
		slog.Warn("doctrine named units this faction cannot build",
			"faction", faction, "doctrine", doctrine.Name, "dropped", dropped)
	}

	prepared, err := s.prepareDoctrine(ctx, doctrine)
	if err != nil {
		slog.Error("doctrine did not compile; keeping the current doctrine", "doctrine", doctrine.Name, "error", err)
		return
	}
	if ctx.Err() != nil || !s.activateDoctrine(generation, doctrine, prepared, events, hasEnemyIntel) {
		slog.Info("discarded stale doctrine", "doctrine", doctrine.Name)
		return
	}
	slog.Info("doctrine activated", "name", doctrine.Name, "rationale", doctrine.Rationale)

}

// UseVimyc sets the compiler doctrines go through. Required — without one a
// doctrine cannot become rules.
func (s *Strategist) UseVimyc(c *rules.VimycCompiler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compiler = c
}
