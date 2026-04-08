package rules

import "fmt"

// addEconomyRules emits rules for power plants, refineries, advanced power,
// ore silos, and extra harvester production.
func (c *doctrineCompiler) addEconomyRules() {
	// --- Economy ---

	powerCashThreshold := lerp(500, 200, c.d.EconomyPriority)
	c.rules = append(c.rules, &Rule{
		Name:         "build-power",
		Priority:     800,
		Category:     "economy",
		Exclusive:    true,
		ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("power_plant") && (PowerExcess() < 0 || RoleCount("power_plant") == 0) && Cash() >= %d`, powerCashThreshold),
		Action:       ActionProducePowerPlant,
	})

	refineryMax := lerp(1, 5, c.d.EconomyPriority)
	refineryCashThreshold := lerp(2000, 800, c.d.EconomyPriority)

	// First refinery — critical economy, highest non-power priority.
	c.rules = append(c.rules, &Rule{
		Name:         "build-refinery",
		Priority:     750,
		Category:     "economy",
		Exclusive:    true,
		ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("refinery") && RoleCount("refinery") < 1 && Cash() >= %d`, refineryCashThreshold),
		Action:       ActionProduceRefinery,
	})

	// Second refinery — strong early-game investment that pays for itself
	// quickly. Lower cash threshold than later refineries so it comes out
	// competitively with built-in AI timing.
	if c.d.EconomyPriority > DoctrineEnabled {
		secondRefPriority := lerp(560, 700, c.d.EconomyPriority)
		secondRefCash := lerp(1500, 500, c.d.EconomyPriority)
		c.rules = append(c.rules, &Rule{
			Name:         "build-second-refinery",
			Priority:     secondRefPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("refinery") && RoleCount("refinery") == 1 && (HasRole("barracks") || HasRole("war_factory")) && Cash() >= %d`, secondRefCash),
			Action:       ActionProduceRefinery,
		})
	}

	// Expansion refineries (3rd+) — priority scales with economy emphasis so
	// military buildings can compete when doctrine favours them.
	if c.d.EconomyPriority > DoctrineEnabled && refineryMax > 2 {
		extraRefPriority := lerp(520, 680, c.d.EconomyPriority)
		c.rules = append(c.rules, &Rule{
			Name:         "build-extra-refinery",
			Priority:     extraRefPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("refinery") && RoleCount("refinery") >= 2 && RoleCount("refinery") < %d && (HasRole("barracks") || HasRole("war_factory")) && Cash() >= %d`, refineryMax, refineryCashThreshold),
			Action:       ActionProduceRefinery,
		})
	}

	// --- Economy scaling ---

	if c.d.EconomyPriority > DoctrineSignificant || c.d.TechPriority > DoctrineDominant {
		c.rules = append(c.rules, &Rule{
			Name:         "build-advanced-power",
			Priority:     790,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("advanced_power") && PowerExcess() < %d && Cash() >= 500`, LowPowerHeadroom),
			Action:       ActionProduceAdvancedPower,
		})
	}

	if c.d.EconomyPriority > DoctrineDominant {
		siloCap := lerp(0, 2, c.d.EconomyPriority)
		c.rules = append(c.rules, &Rule{
			Name:         "build-ore-silo",
			Priority:     300,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("ore_silo") && ResourcesNearCap() && RoleCount("ore_silo") < %d && Cash() >= 150`, siloCap),
			Action:       ActionProduceOreSilo,
		})
	}

	if c.d.EconomyPriority > DoctrineDominant {
		c.rules = append(c.rules, &Rule{
			Name:         "produce-extra-harvester",
			Priority:     420,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("refinery") && HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildRole("harvester") && RoleCount("harvester") < RoleCount("refinery") + 1 && %s`, buildCashCondition(1400, c.savings)),
			Action:       ActionProduceHarvester,
		})
	}
}
