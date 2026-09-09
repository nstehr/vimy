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
	EnemyBuildings     []TypeCount // currently visible
	EnemyBuildingsSeen []TypeCount // cumulative historical
	EnemyUnits         []TypeCount // currently visible
	EnemyUnitsSeen     []TypeCount // cumulative historical
}

// Strategist consults the LLM in the background for a doctrine, then swaps the
// rule engine's rule set.
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

	// prevSnap's ID sets are a high-water mark, so losses accumulate across state
	// updates rather than resetting every tick.
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
	// Situation signals the tempo knobs are read against.
	situation.Ground_squad_ready_ratio = groundSquadReadyRatio(s.engine.Memory, *gs)
	situation.Cash_burn_rate = int64(s.computeCashBurnRate(gs))
	// Named rather than inferred: the prompt's rosters are keyed by side, and the
	// model does not reliably know that germany is Allied.
	situation.Faction_side = rules.SideOf(faction)
	situation.Unbuildable_roles = rules.UnbuildableRoles(faction)
	situation.Time_to_reach_enemy_estimate = int64(timeToReachEnemyEstimate(s.engine.Memory, *gs))
	// A hard gate in the rule layer, because told only in the prompt the model
	// kept committing aircraft into repeated flak counters.
	burnedSet := make(map[string]bool, len(burnedAxes))
	for _, a := range burnedAxes {
		burnedSet[a] = true
	}
	s.engine.Memory["burnedAxes"] = burnedSet
	// Likewise for pressure: the prompt shapes the doctrine, but only the rule
	// layer can move cash floors and rule priorities in response to a rush.
	s.engine.Memory["beingRushed"] = beingRushed
	s.engine.Memory["harvesterHarassed"] = harvesterHarassed
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

	// Dropped before anything reads it: unit selection already skips unbuildable
	// roles, but DoctrineParams reads the raw list and would move rule priorities
	// on the strength of a name.
	var dropped []string
	doctrine, dropped = rules.FilterPreferences(doctrine, faction)
	if len(dropped) > 0 {
		slog.Warn("doctrine named units this faction cannot build",
			"faction", faction, "doctrine", doctrine.Name, "dropped", dropped)
	}

	// Close out the previous window: the flushed counters belong to the
	// DoctrineRecord that was active for them.
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
		// Keep the running rule set. Any fallback would hide exactly the failure
		// this path exists to surface.
		slog.Error("doctrine did not compile; keeping the current rules",
			"doctrine", doctrine.Name, "error", err)
		return
	}
	if err := s.engine.Swap(compiled); err != nil {
		slog.Error("strategist rule swap failed", "error", err)
		return
	}

	// The window's available rule set, to diff later against what fired — which
	// is why it is archived only when tracing is on.
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

// UseVimyc sets the compiler doctrines go through. Required — without one a
// doctrine cannot become rules.
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

// snapshotSuperweaponFires returns fire deltas since the last evaluation and
// re-baselines for the next one.
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

	snapshot := make(map[string]int, len(totalFires))
	for k, v := range totalFires {
		snapshot[k] = v
	}
	memory["superweaponFiresSnapshot"] = snapshot

	return fires
}

// stressEventTTL keeps a high-impact event in the feed for roughly three
// evaluation intervals — the pivot itself plus a follow-up, before the model is
// allowed to consider reverting.
const stressEventTTL = 1500

// stressKinds are the events signalling pressure a single pivot rarely
// resolves. Lower-impact kinds stay out so the persisted feed stays tight.
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

// mergeStressEvents dedupes fresh and persisted events by (Kind, Tick), so the
// LLM sees this tick's events alongside recent unresolved stress.
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

// axisDominantThreshold is where a doctrine counts as pivoting to an axis.
// Mirrors the convergence-break wording in the prompt.
const axisDominantThreshold = 0.55

// burnedAxisMinPivots is how many countered pivots burn an axis. Air needs only
// one: SAM and flak clusters are effectively permanent. The others need two, so
// a single bad engagement — rifles into one tesla — doesn't abandon the axis.
var burnedAxisMinPivots = map[string]int{
	"air":      1,
	"infantry": 2,
	"vehicle":  2,
	"naval":    2,
}

// burnedAxisLookbackTicks is how long after a pivot a counter still counts
// against it — the same window stress events persist for.
const burnedAxisLookbackTicks = 1500

// burnedAxisRecoveryTicks un-burns an axis after a counter-free period; 0 means
// the burn holds for the match. Only air recovers: AA can be suppressed, and
// without recovery one early SAM kill neuters an air-superiority directive for
// the remaining 100k ticks. Ground counter-tech only stacks up over a match.
var burnedAxisRecoveryTicks = map[string]int{
	"air": 5000,
}

// computeBurnedAxes returns axes pivoted to at least burnedAxisMinPivots times,
// each pivot followed by a domain-specific counter event. Downstream this is a
// hard constraint, not a suggestion.
//
// Over the full history rather than a recent window: repeat air pivots are
// routinely 10-20 doctrines apart and a short window catches none of them.
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
		// An axis whose latest counter has aged out of its recovery window
		// un-burns.
		if rt, has := burnedAxisRecoveryTicks[axis]; has && rt > 0 {
			if currentTick-axisLatestCounter[axis] > rt {
				continue
			}
		}
		out = append(out, axis)
	}
	return out
}

// latestAxisCounter returns the tick of the most recent matching stress event
// within the lookback window after pivotTick.
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

// eventMatchesAxis is shared by latestAxisCounter and axisCounteredAfter.
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

// axisCounteredAfter reports whether a matching stress event fired inside the
// lookback window after a pivot. Matching is on the detail string, which is
// where strategy_countered events carry their domain.
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

// Pressure-flag thresholds. Harvesters under attack is not one signal: in a
// rush they always are, because there is no perimeter yet. The two cases want
// opposite responses, so they are separate flags:
//
//   - being_rushed: early game. Counter-force — killing the rushers is what
//     unblocks the economy, not bunkering.
//   - harvester_harassed: sustained losses against an established base.
//     Dispersal, escorts and static defense, at some cost to the main push.
const (
	// Wide enough for slow-paced rushes: a first harvester attack near tick
	// 10000 against a still-small base is a rush, and a tighter window sends it
	// to the softer harassment response instead.
	rushTickCutoff = 10500
	// No building-count gate: vimy routinely clears any plausible threshold
	// before raids arrive, so the condition only made the flag silently false.
	// The tick window already means "early game", and the rush rules cap their
	// own output, so leaving the flag on for the window can't run away.
	harassTickFloor          = 10500
	harassMinHarvesterEvents = 2    // sustained pressure, not one-off
	pressureLookbackTicks    = 2000 // "recent" stress events window
)

// computePressureFlags returns (beingRushed, harvesterHarassed). Both can be
// false; the disjoint tick thresholds keep both from being true at once, though
// the prompt handles each independently.
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

// groundSquadReadyRatio is the ground-attack squad's idle/target ratio, 0 when
// it hasn't formed. Read against the doctrine's commit_ratio so the LLM can see
// whether its intended commit threshold is reachable.
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

// computeCashBurnRate is the net cash delta since the last evaluation, positive
// when income exceeds spending. Zero on the first, and re-baselines as a side
// effect.
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

// timeToReachEnemyEstimate is straight-line distance to the nearest known enemy
// base at an approximate walking pace, or -1 with no intel.
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
	// A rifle walks ~0.25 cells per tick in RA.
	const ticksPerMapUnit = 4.0
	return int(best * ticksPerMapUnit)
}

// recentDoctrineSummaries fingerprints the last n doctrines for the prompt, so
// the LLM can see its own attempts converging on a shape the battlefield keeps
// rejecting — a pivot signal that lessons alone don't produce.
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

// buildSituation assembles the GameSituation handed to the LLM. No side effects
// beyond reading memory.
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

	// Currently visible enemies.
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

	buildingCounts := make(map[string]int)
	for _, b := range gs.Buildings {
		buildingCounts[b.Type]++
	}
	for t, c := range buildingCounts {
		sit.Buildings = append(sit.Buildings, types.TypeCount{Type: t, Count: int64(c)})
	}

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

	for t, c := range rules.GetEnemyUnitsSeen(memory) {
		sit.Enemy_units_seen = append(sit.Enemy_units_seen, types.TypeCount{Type: t, Count: int64(c)})
	}
	for t, c := range rules.GetEnemyBuildingsSeen(memory) {
		sit.Enemy_buildings_seen = append(sit.Enemy_buildings_seen, types.TypeCount{Type: t, Count: int64(c)})
	}

	// Without these the strategist has no reason to raise capture_priority when
	// scouts turn up a tech building.
	if len(gs.Capturables) > 0 {
		capCounts := make(map[string]int)
		for _, c := range gs.Capturables {
			capCounts[c.Type]++
		}
		for t, c := range capCounts {
			sit.Capturables_visible = append(sit.Capturables_visible, types.TypeCount{Type: t, Count: int64(c)})
		}
	}

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

	for _, e := range events {
		sit.Recent_events = append(sit.Recent_events, types.GameEvent{
			Kind:   string(e.Kind),
			Tick:   int64(e.Tick),
			Detail: e.Detail,
		})
	}

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
