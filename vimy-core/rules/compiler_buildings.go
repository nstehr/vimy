package rules

import "fmt"

// addBuildingRules emits rules for prerequisite buildings (radar, barracks,
// war factory), military production buildings (airfield, naval yard, service
// depot), defenses (ground, AA, gap generator), tech progression, superweapon
// buildings, and extra production buildings.
func (c *doctrineCompiler) addBuildingRules() {
	// --- Prerequisite buildings ---

	// Radar is the tech-tree gate for vehicles, aircraft, and naval —
	// include it whenever any of those paths are desired. Requires at
	// least one military building so it doesn't jump ahead of barracks.
	if c.d.VehicleWeight > DoctrineEnabled || c.d.AirWeight > DoctrineEnabled || c.d.NavalWeight > DoctrineEnabled || c.d.TechPriority > DoctrineSignificant {
		c.rules = append(c.rules, &Rule{
			Name:         "build-radar",
			Priority:     710,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("radar") && !HasRole("radar") && !QueueProducingRole("radar") && (HasRole("barracks") || HasRole("war_factory")) && PowerExcess() >= 0 && Cash() >= 1000`,
			Action:       ActionProduceRadar,
		})
	}

	// --- Military buildings ---
	// Priorities scale with weight so the doctrine's emphasis determines
	// build order (e.g. air-heavy → airfield before war factory).

	// needsBarracks tracks whether barracks is already included by the
	// infantry/defense path. If radar is included but barracks isn't,
	// we add it as a prerequisite building (barracks is the cheapest
	// tech-tree gate to radar in RA).
	barracksIncluded := false

	if c.d.InfantryWeight > DoctrineEnabled || c.d.GroundDefensePriority > DoctrineModerate {
		barracksIncluded = true
		barracksPriority := lerp(600, 700, c.d.InfantryWeight)
		if c.d.GroundDefensePriority > DoctrineModerate {
			barracksPriority = max(barracksPriority, lerp(600, 700, c.d.GroundDefensePriority))
		}
		c.rules = append(c.rules, &Rule{
			Name:         "build-barracks",
			Priority:     barracksPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("barracks") && !HasRole("barracks") && PowerExcess() >= 0 && Cash() >= 300`,
			Action:       ActionProduceBarracks,
		})
	}

	if c.d.VehicleWeight > DoctrineEnabled {
		warFactoryPriority := lerp(580, 680, c.d.VehicleWeight)
		// Scale cash threshold inversely with vehicle weight: low-vehicle
		// doctrines need a bigger buffer so the 2000-credit building doesn't
		// starve air/naval production during construction. Ceiling capped at
		// 2500 (was 3500) — the old value made it nearly impossible for rush
		// doctrines to accumulate enough cash since infantry production drained
		// funds below the threshold.
		wfCashThreshold := lerp(2500, 2000, c.d.VehicleWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "build-war-factory",
			Priority:     warFactoryPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("war_factory") && !HasRole("war_factory") && PowerExcess() >= 0 && Cash() >= %d`, wfCashThreshold),
			Action:       ActionProduceWarFactory,
		})
	}

	// If radar is included but no military building rule exists yet,
	// add barracks as a prerequisite — it's the cheapest tech-tree gate
	// to radar in RA and prevents a deadlock where radar can never be built.
	radarIncluded := c.d.VehicleWeight > DoctrineEnabled || c.d.AirWeight > DoctrineEnabled || c.d.NavalWeight > DoctrineEnabled || c.d.TechPriority > DoctrineSignificant
	if radarIncluded && !barracksIncluded && c.d.VehicleWeight <= DoctrineEnabled {
		c.rules = append(c.rules, &Rule{
			Name:         "build-barracks-prereq",
			Priority:     600,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("barracks") && !HasRole("barracks") && PowerExcess() >= 0 && Cash() >= 300`,
			Action:       ActionProduceBarracks,
		})
	}

	if c.d.AirWeight > DoctrineEnabled {
		airfieldPriority := lerp(580, 680, c.d.AirWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "build-airfield",
			Priority:     airfieldPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("airfield") && !HasRole("airfield") && PowerExcess() >= 0 && Cash() >= 500`,
			Action:       ActionProduceAirfield,
		})
	}

	if c.d.VehicleWeight > DoctrineSignificant {
		c.rules = append(c.rules, &Rule{
			Name:         "build-service-depot",
			Priority:     570,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("service_depot") && !HasRole("service_depot") && HasRole("war_factory") && PowerExcess() >= 0 && Cash() >= 1200`,
			Action:       ActionProduceServiceDepot,
		})
	}

	if c.d.NavalWeight > DoctrineEnabled {
		navalYardPriority := lerp(580, 680, c.d.NavalWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "build-naval-yard",
			Priority:     navalYardPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `MapHasWater() && !QueueBusy("Building") && CanBuildRole("naval_yard") && !HasRole("naval_yard") && PowerExcess() >= 0 && Cash() >= 500`,
			Action:       ActionProduceNavalYard,
		})
	}

	// --- Ground defenses ---

	if c.d.GroundDefensePriority > DoctrineModerate {
		defenseCap := lerp(1, 5, c.d.GroundDefensePriority)
		defenseCash := lerp(1500, 300, c.d.GroundDefensePriority)
		defensePriority := lerp(400, 600, c.d.GroundDefensePriority)
		c.rules = append(c.rules, &Rule{
			Name:         "build-base-defense",
			Priority:     defensePriority,
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Defense") && PowerExcess() >= 0 && (CanBuildRole("pillbox") || CanBuildRole("camo_pillbox") || CanBuildRole("turret") || CanBuildRole("flame_tower") || CanBuildRole("tesla_coil")) && (RoleCount("pillbox") + RoleCount("camo_pillbox") + RoleCount("turret") + RoleCount("flame_tower") + RoleCount("tesla_coil")) < %d && Cash() >= %d`, defenseCap, defenseCash),
			Action:       ActionProduceDefense,
		})
	}

	// --- AA defenses ---

	if c.d.AirDefensePriority > DoctrineSignificant {
		aaCap := lerp(1, 3, c.d.AirDefensePriority)
		aaCash := lerp(1200, 500, c.d.AirDefensePriority)
		aaPriority := lerp(400, 600, c.d.AirDefensePriority)
		c.rules = append(c.rules, &Rule{
			Name:         "build-aa-defense",
			Priority:     aaPriority,
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Defense") && PowerExcess() >= 0 && CanBuildRole("aa_defense") && RoleCount("aa_defense") < %d && Cash() >= %d`, aaCap, aaCash),
			Action:       ActionProduceAADefense,
		})
	}

	// --- Gap generator (Allied strategic defense) ---

	if c.d.GroundDefensePriority > DoctrineSignificant && c.d.TechPriority > DoctrineSignificant {
		gapCap := lerp(1, 2, c.d.GroundDefensePriority)
		c.rules = append(c.rules, &Rule{
			Name:         "build-gap-generator",
			Priority:     lerp(400, 550, c.d.GroundDefensePriority),
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Defense") && PowerExcess() >= 0 && CanBuildRole("gap_generator") && HasRole("tech_center") && RoleCount("gap_generator") < %d && Cash() >= 800`, gapCap),
			Action:       ActionProduceGapGenerator,
		})
	}

	// --- Tech progression ---

	if c.d.TechPriority > DoctrineHigh {
		techCenterPriority := lerp(600, 660, c.d.TechPriority)
		c.rules = append(c.rules, &Rule{
			Name:         "build-tech-center",
			Priority:     techCenterPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("tech_center") && !HasRole("tech_center") && HasRole("radar") && PowerExcess() >= 0 && Cash() >= 1500`,
			Action:       ActionProduceTechCenter,
		})
	}

	// --- Superweapon buildings ---

	if c.d.SuperweaponPriority > DoctrineSignificant {
		c.rules = append(c.rules, &Rule{
			Name:         "build-missile-silo",
			Priority:     650,
			Category:     "superweapon_build",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Defense") && CanBuildRole("missile_silo") && !HasRole("missile_silo") && HasRole("tech_center") && PowerExcess() >= 0 && Cash() >= 2500`,
			Action:       ActionProduceMissileSilo,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "build-iron-curtain",
			Priority:     640,
			Category:     "superweapon_build",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Defense") && CanBuildRole("iron_curtain") && !HasRole("iron_curtain") && HasRole("tech_center") && PowerExcess() >= 0 && Cash() >= 2500`,
			Action:       ActionProduceIronCurtain,
		})
	}

	// --- Extra production buildings ---

	if c.d.InfantryWeight > DoctrineExtreme {
		extraBarracksCap := lerp(1, 3, c.d.InfantryWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "build-extra-barracks",
			Priority:     500,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("barracks") && RoleCount("barracks") < %d && PowerExcess() >= 0 && Cash() >= 300`, extraBarracksCap),
			Action:       ActionProduceBarracks,
		})
	}

	if c.d.VehicleWeight > DoctrineExtreme {
		extraWFCap := lerp(1, 2, c.d.VehicleWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "build-extra-war-factory",
			Priority:     490,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("war_factory") && RoleCount("war_factory") < %d && PowerExcess() >= 0 && Cash() >= 2000`, extraWFCap),
			Action:       ActionProduceWarFactory,
		})
	}

	if c.d.AirWeight > DoctrineExtreme {
		extraAirCap := lerp(1, 3, c.d.AirWeight)
		// Only build extra airfields once existing ones are fully utilized
		// (aircraft count near cap). Otherwise the building drains cash
		// that should go to unit production at the existing airfield.
		airCapForGate := lerp(2, 8, c.d.AirWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "build-extra-airfield",
			Priority:     480,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("airfield") && RoleCount("airfield") < %d && CombatAircraftCount() >= %d && PowerExcess() >= 0 && Cash() >= 500`, extraAirCap, airCapForGate-1),
			Action:       ActionProduceAirfield,
		})
	}

	if c.d.NavalWeight > DoctrineExtreme {
		extraNavalCap := lerp(1, 2, c.d.NavalWeight)
		// Same logic: only expand naval yards when existing capacity is used.
		navalCapForGate := lerp(3, 8, c.d.NavalWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "build-extra-naval-yard",
			Priority:     470,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && !QueueBusy("Building") && CanBuildRole("naval_yard") && RoleCount("naval_yard") < %d && (RoleCount("submarine") + RoleCount("destroyer")) >= %d && PowerExcess() >= 0 && Cash() >= 500`, extraNavalCap, navalCapForGate-1),
			Action:       ActionProduceNavalYard,
		})
	}
}
