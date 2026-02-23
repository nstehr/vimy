package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
		ready:     make(chan struct{}, 1),
	}
}

// SetFaction sets the faction string (called from HandleHello).
func (s *Strategist) SetFaction(f string) {
	s.mu.Lock()
	s.faction = f
	s.mu.Unlock()
}

// UpdateState stores the latest game state. Signals readiness on the first call
// and on interval boundaries.
func (s *Strategist) UpdateState(gs model.GameState) {
	s.mu.Lock()
	first := s.latest == nil
	s.latest = &gs
	shouldSignal := first || (gs.Tick-s.lastTick >= s.interval)
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
	s.mu.Unlock()

	if gs == nil {
		return
	}

	situation := summarize(*gs, s.engine.Memory)
	slog.Debug("strategist evaluating", "tick", gs.Tick, "directive", s.directive)

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
	}
}

// summarize produces a human-readable text summary of game state for the LLM.
func summarize(gs model.GameState, memory map[string]any) string {
	var b strings.Builder

	// Phase heuristic
	phase := "Early Game"
	if gs.Tick > 3000 {
		phase = "Late Game"
	} else if gs.Tick > 1000 {
		phase = "Mid Game"
	}

	fmt.Fprintf(&b, "Tick: %d | Phase: %s\n", gs.Tick, phase)
	fmt.Fprintf(&b, "Cash: %d | Resources: %d/%d\n",
		gs.Player.Cash, gs.Player.Resources, gs.Player.ResourceCapacity)
	fmt.Fprintf(&b, "Power: %d/%d (%s)\n",
		gs.Player.PowerDrained, gs.Player.PowerProvided, gs.Player.PowerState)

	// Buildings summary
	buildingCounts := make(map[string]int)
	for _, b := range gs.Buildings {
		buildingCounts[b.Type]++
	}
	if len(buildingCounts) > 0 {
		fmt.Fprintf(&b, "Buildings:")
		for t, c := range buildingCounts {
			fmt.Fprintf(&b, " %dx %s", c, t)
		}
		fmt.Fprintln(&b)
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
	if len(unitCounts) > 0 {
		fmt.Fprintf(&b, "Units:")
		for t, c := range unitCounts {
			fmt.Fprintf(&b, " %dx %s", c, t)
		}
		fmt.Fprintf(&b, " (%d idle)\n", idleCount)
	}

	// Production queues
	for _, pq := range gs.ProductionQueues {
		if pq.CurrentItem != "" {
			fmt.Fprintf(&b, "Queue %s: producing %s (%d%%)\n", pq.Type, pq.CurrentItem, pq.CurrentProgress)
		}
	}

	// Enemies
	fmt.Fprintf(&b, "Enemies visible: %d\n", len(gs.Enemies))

	// Enemy intel
	if bases, ok := memory["enemyBases"].(map[string]rules.EnemyBaseIntel); ok && len(bases) > 0 {
		for owner, base := range bases {
			fmt.Fprintf(&b, "Known enemy base [%s]: (%d, %d) last seen tick %d\n", owner, base.X, base.Y, base.Tick)
		}
	} else {
		fmt.Fprintln(&b, "Known enemy bases: none (not yet scouted)")
	}

	// Map
	fmt.Fprintf(&b, "Map: %dx%d\n", gs.MapWidth, gs.MapHeight)

	return b.String()
}
