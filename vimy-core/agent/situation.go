package agent

import (
	"fmt"
	"math"

	"github.com/nstehr/vimy/vimy-core/baml_client/types"
	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
)

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
		ForceSize:                 d.Force_size,
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

// groundSquadReadyRatio is how much of the ground-attack squad's intended force
// is alive and present, 0 when it hasn't formed. Read against the doctrine's
// commit_ratio so the LLM can see whether its commit threshold is reachable.
//
// Must agree with rules.SquadReadyRatio, which the gate actually uses. It did
// not: both counted units flagged Idle, and Idle means "has no current order",
// so the number shown to the strategist went to zero the moment the squad was
// given one. The strategist was being asked to compare its commit_ratio against
// a figure that measured standing still.
func groundSquadReadyRatio(memory map[string]any, gs model.GameState) float64 {
	squads, ok := memory["squads"].(map[string]*rules.Squad)
	if !ok {
		return 0
	}
	sq, ok := squads["ground-attack"]
	if !ok || sq.TargetSize <= 0 {
		return 0
	}
	alive := make(map[int]bool, len(gs.Units))
	for _, u := range gs.Units {
		alive[u.ID] = true
	}
	present := 0
	for _, id := range sq.UnitIDs {
		if alive[id] {
			present++
		}
	}
	if present >= sq.TargetSize {
		return 1
	}
	return float64(present) / float64(sq.TargetSize)
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

	// The strategist is shown both force lists and, until 2026-09-10, was never
	// asked to compare them. It picked economy_priority 0.9 with vehicle_weight
	// 0.25 while fielding nine harvesters against five soldiers, game after
	// game. Counting items in a rendered list is exactly what a model does
	// badly, so the ratio is computed here.
	ours := 0
	for _, u := range gs.Units {
		if rules.IsCombatUnit(u.Type) {
			ours++
		}
	}
	sit.Our_combat_units = int64(ours)
	if n := len(gs.Units); n > 0 {
		harv := 0
		for _, u := range gs.Units {
			if rules.IsHarvester(u.Type) {
				harv++
			}
		}
		sit.Harvester_percent = int64(100 * harv / n)
	}

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
				Phase:      rules.RuleEnv{Memory: memory}.AssaultPhase(sq.Name),
			})
		}
	}

	enemyCombat := 0
	for t, c := range rules.GetEnemyUnitsSeen(memory) {
		sit.Enemy_units_seen = append(sit.Enemy_units_seen, types.TypeCount{Type: t, Count: int64(c)})
		// Cumulative, so this counts units already destroyed. That is the right
		// measure for the comparison: it says what the opponent has been
		// willing to build, not what happens to be alive this second.
		if rules.IsCombatUnit(t) {
			enemyCombat += c
		}
	}
	sit.Enemy_combat_units_seen = int64(enemyCombat)
	for t, c := range rules.GetEnemyBuildingsSeen(memory) {
		sit.Enemy_buildings_seen = append(sit.Enemy_buildings_seen, types.TypeCount{Type: t, Count: int64(c)})
	}

	// Without these the strategist has no reason to raise capture_priority when
	// scouts turn up a tech building.
	//
	// Filtered, because GameState.Capturables is not a list of tech buildings.
	// It carries whatever something could take: in one game the enemy's
	// construction yard, refineries, power, a SAM site and a flame tower, plus
	// their APCs, flak trucks, heavy tanks, harvesters and a tank husk,
	// alongside four oil derricks. Unfiltered it rendered as "Capturable
	// neutral buildings visible: 2x apc 1x 3tnk 1x ftrk", which asked the
	// strategist to spend on engineers to go and capture enemy armour.
	// Remembered, not visible. A derrick does not move: the count used to spike
	// as a scout drove past one and fall back to zero as it drove on, so
	// capture_priority was being set from whatever happened to be on screen at
	// the instant the strategist ran.
	capCounts := make(map[string]int)
	for _, c := range rules.RememberedCapturables(memory) {
		capCounts[rules.BaseTypeName(c.Type)]++
	}
	for t, c := range capCounts {
		sit.Capturables_visible = append(sit.Capturables_visible, types.TypeCount{Type: t, Count: int64(c)})
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

	// Kills alone are worth reporting: a game where we have lost nothing and
	// killed nothing is not the same as one where we have lost nothing because
	// we are winning.
	if len(totalLosses) > 0 || gs.Player.UnitsKilled > 0 {
		sit.Combat_stats = &types.CombatStats{
			Infantry_lost: int64(totalLosses["infantry"]),
			Vehicles_lost: int64(totalLosses["vehicle"]),
			Aircraft_lost: int64(totalLosses["aircraft"]),
			Naval_lost:    int64(totalLosses["naval"]),

			// The engine's own count, not the sidecar's inference. Game 118
			// inferred 396 kills where the engine recorded 138, and 10 enemy
			// buildings where it recorded none — the strategist was being told
			// it was winning fights it was losing.
			Enemy_units_killed:        int64(gs.Player.UnitsKilled),
			Enemy_buildings_destroyed: int64(gs.Player.BuildingsKilled),
		}
	}

	return sit
}
