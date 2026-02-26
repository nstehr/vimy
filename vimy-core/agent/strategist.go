package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	baml_client "github.com/nstehr/vimy/vimy-core/baml_client"
	"github.com/nstehr/vimy/vimy-core/baml_client/types"
	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
)

// Strategist runs in the background, periodically consulting the LLM
// to generate a doctrine and swap the rule engine's rule set.
type Strategist struct {
	mu        sync.Mutex
	latest    *model.GameState
	engine    *rules.Engine
	faction   string
	directive string // initial doctrine seed from --doctrine flag
	interval  int    // re-evaluate every N ticks
	lastTick  int    // tick of last evaluation
	ready     chan struct{}
	prevSnap  *stateSnapshot // previous state snapshot for event diff
	cooldown  int            // minimum ticks between event-driven evaluations
	pending   []Event        // events accumulated since last evaluation
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

// SetFaction sets the faction string (called from HandleHello).
func (s *Strategist) SetFaction(f string) {
	s.mu.Lock()
	s.faction = f
	s.mu.Unlock()
}

// UpdateState stores the latest game state, detects events, and signals
// readiness on the first call, interval boundaries, or significant events.
func (s *Strategist) UpdateState(gs model.GameState) {
	s.mu.Lock()
	first := s.latest == nil
	s.latest = &gs

	// Detect events against the previous snapshot.
	// prevSnap's domain ID sets are accumulated (high-water mark) so that
	// losses add up across multiple state updates instead of resetting each tick.
	events := detectEvents(gs, s.engine.Memory, s.prevSnap)
	snap := takeSnapshot(gs, s.engine.Memory)

	if s.prevSnap != nil {
		snap.lastCounterTick = s.prevSnap.lastCounterTick

		counterFired := false
		for _, e := range events {
			if e.Kind == EventStrategyCountered {
				snap.lastCounterTick = gs.Tick
				counterFired = true
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
	s.mu.Unlock()

	if gs == nil {
		return
	}

	for _, e := range events {
		slog.Info("event detected", "kind", e.Kind, "tick", e.Tick, "detail", e.Detail)
	}
	slog.Debug("strategist evaluating", "tick", gs.Tick, "directive", s.directive, "events", len(events))

	swFires := snapshotSuperweaponFires(s.engine.Memory)
	situation := buildSituation(*gs, s.engine.Memory, events, swFires)

	bamlDoctrine, err := baml_client.GenerateDoctrine(ctx, s.directive, situation, faction)
	if err != nil {
		slog.Error("strategist LLM call failed", "error", err)
		return
	}

	doctrine := fromBAML(bamlDoctrine)
	doctrine.Validate()

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
	)

	compiled := rules.CompileDoctrine(doctrine)
	if err := s.engine.Swap(compiled); err != nil {
		slog.Error("strategist rule swap failed", "error", err)
		return
	}

	s.mu.Lock()
	s.lastTick = gs.Tick
	s.mu.Unlock()
}

// fromBAML converts the BAML-generated Doctrine type to our rules.Doctrine.
func fromBAML(d types.Doctrine) rules.Doctrine {
	return rules.Doctrine{
		Name:                  d.Name,
		Rationale:             d.Rationale,
		EconomyPriority:       d.Economy_priority,
		Aggression:            d.Aggression,
		GroundDefensePriority: d.Ground_defense_priority,
		AirDefensePriority:    d.Air_defense_priority,
		TechPriority:          d.Tech_priority,
		InfantryWeight:        d.Infantry_weight,
		VehicleWeight:         d.Vehicle_weight,
		AirWeight:             d.Air_weight,
		NavalWeight:           d.Naval_weight,
		GroundAttackGroupSize: int(d.Ground_attack_group_size),
		AirAttackGroupSize:    int(d.Air_attack_group_size),
		NavalAttackGroupSize:  int(d.Naval_attack_group_size),
		ScoutPriority:             d.Scout_priority,
		SpecializedInfantryWeight: d.Specialized_infantry_weight,
		SuperweaponPriority:       d.Superweapon_priority,
		CapturePriority:           d.Capture_priority,
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

// buildSituation constructs a structured GameSituation from the current
// game state, memory, events, and superweapon fire data. Pure function
// (aside from reading memory) — no side effects.
func buildSituation(gs model.GameState, memory map[string]any, events []Event, swFires []types.SuperweaponFire) types.GameSituation {
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

	return sit
}
