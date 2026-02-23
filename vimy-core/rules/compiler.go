package rules

import "fmt"

// CompileDoctrine generates a complete rule set from a doctrine's weights.
// All conditions are built via fmt.Sprintf with interpolated values —
// the compiler never generates invalid expr.
func CompileDoctrine(d Doctrine) []*Rule {
	d.Validate()
	var rules []*Rule

	// --- Core rules (always present) ---

	rules = append(rules, &Rule{
		Name:         "deploy-mcv",
		Priority:     1000,
		Category:     "setup",
		Exclusive:    true,
		ConditionSrc: `HasUnit("mcv") && !HasRole("construction_yard")`,
		Action:       ActionDeployMCV,
	})

	rules = append(rules, &Rule{
		Name:         "recover-mcv",
		Priority:     950,
		Category:     "setup",
		Exclusive:    true,
		ConditionSrc: `!HasRole("construction_yard") && !HasUnit("mcv") && HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuild("Vehicle","mcv") && Cash() >= 1000`,
		Action:       ActionProduceMCV,
	})

	rules = append(rules, &Rule{
		Name:         "place-ready-building",
		Priority:     900,
		Category:     "economy",
		Exclusive:    true,
		ConditionSrc: `QueueReady("Building")`,
		Action:       ActionPlaceBuilding,
	})

	rules = append(rules, &Rule{
		Name:         "place-ready-defense",
		Priority:     895,
		Category:     "defense",
		Exclusive:    true,
		ConditionSrc: `QueueReady("Defense")`,
		Action:       ActionPlaceDefense,
	})

	rules = append(rules, &Rule{
		Name:         "capture-building",
		Priority:     850,
		Category:     "economy",
		Exclusive:    false,
		ConditionSrc: `CapturableCount() > 0 && len(IdleEngineers()) > 0`,
		Action:       ActionCaptureBuilding,
	})

	rules = append(rules, &Rule{
		Name:         "produce-engineer",
		Priority:     550,
		Category:     "production",
		Exclusive:    false,
		ConditionSrc: `CapturableCount() > 0 && !QueueBusy("Infantry") && CanBuildRole("engineer") && RoleCount("engineer") < CapturableCount() && Cash() >= 500`,
		Action:       ActionProduceEngineer,
	})

	// --- Rebuild rules (always present, high priority) ---
	// These fire when a previously-built building is destroyed, using the
	// exclusive "rebuild" category so only one rebuild queues per tick.
	// Harvester rebuild uses the Vehicle queue (not Building), but shares
	// the category so only one rebuild decision is made per tick.

	rules = append(rules, &Rule{
		Name:         "rebuild-harvester",
		Priority:     830,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `HasRole("refinery") && RoleCount("harvester") < RoleCount("refinery") && !QueueBusy("Vehicle") && CanBuildRole("harvester") && Cash() >= 600`,
		Action:       ActionProduceHarvester,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-refinery",
		Priority:     825,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("refinery") && !QueueBusy("Building") && CanBuildRole("refinery") && Cash() >= 500`,
		Action:       ActionProduceRefinery,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-barracks",
		Priority:     820,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("barracks") && !QueueBusy("Building") && CanBuildRole("barracks") && Cash() >= 200`,
		Action:       ActionProduceBarracks,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-war-factory",
		Priority:     815,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("war_factory") && !QueueBusy("Building") && CanBuildRole("war_factory") && Cash() >= 1000`,
		Action:       ActionProduceWarFactory,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-radar",
		Priority:     810,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("radar") && !QueueBusy("Building") && CanBuildRole("radar") && Cash() >= 500`,
		Action:       ActionProduceRadar,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-tech-center",
		Priority:     805,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("tech_center") && !QueueBusy("Building") && CanBuildRole("tech_center") && HasRole("radar") && Cash() >= 1000`,
		Action:       ActionProduceTechCenter,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-airfield",
		Priority:     800,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("airfield") && !QueueBusy("Building") && CanBuildRole("airfield") && Cash() >= 300`,
		Action:       ActionProduceAirfield,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-naval-yard",
		Priority:     800,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("naval_yard") && !QueueBusy("Building") && CanBuildRole("naval_yard") && Cash() >= 300`,
		Action:       ActionProduceNavalYard,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-service-depot",
		Priority:     795,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("service_depot") && !QueueBusy("Building") && CanBuildRole("service_depot") && Cash() >= 800`,
		Action:       ActionProduceServiceDepot,
	})

	rules = append(rules, &Rule{
		Name:         "repair-buildings",
		Priority:     200,
		Category:     "maintenance",
		Exclusive:    false,
		ConditionSrc: `len(DamagedBuildings()) > 0`,
		Action:       ActionRepairDamagedBuildings,
	})

	rules = append(rules, &Rule{
		Name:         "return-idle-harvesters",
		Priority:     100,
		Category:     "harvester",
		Exclusive:    false,
		ConditionSrc: `len(IdleHarvesters()) > 0`,
		Action:       ActionSendIdleHarvesters,
	})

	// --- Economy rules (parameterized by EconomyPriority) ---

	powerCashThreshold := lerp(500, 200, d.EconomyPriority)
	rules = append(rules, &Rule{
		Name:         "build-power",
		Priority:     800,
		Category:     "economy",
		Exclusive:    true,
		ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("power_plant") && (PowerExcess() < 0 || RoleCount("power_plant") == 0) && Cash() >= %d`, powerCashThreshold),
		Action:       ActionProducePowerPlant,
	})

	refineryMax := lerp(1, 5, d.EconomyPriority)
	refineryCashThreshold := lerp(2000, 800, d.EconomyPriority)
	rules = append(rules, &Rule{
		Name:         "build-refinery",
		Priority:     750,
		Category:     "economy",
		Exclusive:    true,
		ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("refinery") && RoleCount("refinery") < %d && Cash() >= %d`, refineryMax, refineryCashThreshold),
		Action:       ActionProduceRefinery,
	})

	// --- Prerequisite buildings ---

	// Radar dome unlocks airfield, war factory, naval yard, and higher-tier units.
	// Build it when any of those paths are desired.
	if d.VehicleWeight > 0.1 || d.AirWeight > 0.1 || d.NavalWeight > 0.1 || d.TechPriority > 0.3 {
		rules = append(rules, &Rule{
			Name:         "build-radar",
			Priority:     710,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("radar") && !HasRole("radar") && PowerExcess() >= 0 && Cash() >= 1000`,
			Action:       ActionProduceRadar,
		})
	}

	// --- Military building rules (enabled by unit weights) ---
	// Priorities scale with corresponding weight so the doctrine's emphasis
	// determines build order (e.g. air-heavy → airfield before war factory).

	if d.InfantryWeight > 0.1 {
		barracksPriority := lerp(600, 700, d.InfantryWeight)
		rules = append(rules, &Rule{
			Name:         "build-barracks",
			Priority:     barracksPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("barracks") && !HasRole("barracks") && PowerExcess() >= 0 && Cash() >= 300`,
			Action:       ActionProduceBarracks,
		})
	}

	if d.VehicleWeight > 0.1 {
		warFactoryPriority := lerp(580, 680, d.VehicleWeight)
		rules = append(rules, &Rule{
			Name:         "build-war-factory",
			Priority:     warFactoryPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("war_factory") && !HasRole("war_factory") && PowerExcess() >= 0 && Cash() >= 2000`,
			Action:       ActionProduceWarFactory,
		})
	}

	if d.AirWeight > 0.1 {
		airfieldPriority := lerp(580, 680, d.AirWeight)
		rules = append(rules, &Rule{
			Name:         "build-airfield",
			Priority:     airfieldPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("airfield") && !HasRole("airfield") && PowerExcess() >= 0 && Cash() >= 500`,
			Action:       ActionProduceAirfield,
		})
	}

	if d.VehicleWeight > 0.3 {
		rules = append(rules, &Rule{
			Name:         "build-service-depot",
			Priority:     570,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("service_depot") && !HasRole("service_depot") && HasRole("war_factory") && PowerExcess() >= 0 && Cash() >= 1200`,
			Action:       ActionProduceServiceDepot,
		})
	}

	if d.NavalWeight > 0.1 {
		navalYardPriority := lerp(540, 620, d.NavalWeight)
		rules = append(rules, &Rule{
			Name:         "build-naval-yard",
			Priority:     navalYardPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("naval_yard") && !HasRole("naval_yard") && PowerExcess() >= 0 && Cash() >= 500`,
			Action:       ActionProduceNavalYard,
		})
	}

	// --- Ground defensive structures (gated by GroundDefensePriority) ---

	if d.GroundDefensePriority > 0.2 {
		defenseCap := lerp(1, 5, d.GroundDefensePriority)
		defenseCash := lerp(1500, 600, d.GroundDefensePriority)
		defensePriority := lerp(400, 600, d.GroundDefensePriority)
		rules = append(rules, &Rule{
			Name:         "build-base-defense",
			Priority:     defensePriority,
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Defense") && PowerExcess() >= 0 && (CanBuildRole("pillbox") || CanBuildRole("turret") || CanBuildRole("flame_tower") || CanBuildRole("tesla_coil")) && (RoleCount("pillbox") + RoleCount("turret") + RoleCount("flame_tower") + RoleCount("tesla_coil")) < %d && Cash() >= %d`, defenseCap, defenseCash),
			Action:       ActionProduceDefense,
		})
	}

	// --- AA defensive structures (gated by AirDefensePriority) ---

	if d.AirDefensePriority > 0.3 {
		aaCap := lerp(1, 3, d.AirDefensePriority)
		aaPriority := lerp(400, 600, d.AirDefensePriority)
		rules = append(rules, &Rule{
			Name:         "build-aa-defense",
			Priority:     aaPriority,
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Defense") && PowerExcess() >= 0 && CanBuildRole("aa_defense") && RoleCount("aa_defense") < %d && Cash() >= 800`, aaCap),
			Action:       ActionProduceAADefense,
		})
	}

	// --- Tech progression (gated by TechPriority) ---

	if d.TechPriority > 0.4 {
		techCenterPriority := lerp(600, 660, d.TechPriority)
		rules = append(rules, &Rule{
			Name:         "build-tech-center",
			Priority:     techCenterPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("tech_center") && !HasRole("tech_center") && HasRole("radar") && PowerExcess() >= 0 && Cash() >= 1500`,
			Action:       ActionProduceTechCenter,
		})
	}

	// --- Extra production buildings (gated by high unit weights) ---

	if d.InfantryWeight > 0.6 {
		extraBarracksCap := lerp(1, 3, d.InfantryWeight)
		rules = append(rules, &Rule{
			Name:         "build-extra-barracks",
			Priority:     500,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("barracks") && RoleCount("barracks") < %d && PowerExcess() >= 0 && Cash() >= 300`, extraBarracksCap),
			Action:       ActionProduceBarracks,
		})
	}

	if d.VehicleWeight > 0.6 {
		extraWFCap := lerp(1, 2, d.VehicleWeight)
		rules = append(rules, &Rule{
			Name:         "build-extra-war-factory",
			Priority:     490,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("war_factory") && RoleCount("war_factory") < %d && PowerExcess() >= 0 && Cash() >= 2000`, extraWFCap),
			Action:       ActionProduceWarFactory,
		})
	}

	if d.AirWeight > 0.6 {
		extraAirCap := lerp(1, 3, d.AirWeight)
		rules = append(rules, &Rule{
			Name:         "build-extra-airfield",
			Priority:     480,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("airfield") && RoleCount("airfield") < %d && PowerExcess() >= 0 && Cash() >= 500`, extraAirCap),
			Action:       ActionProduceAirfield,
		})
	}

	// --- Economy scaling (gated by EconomyPriority) ---

	if d.EconomyPriority > 0.3 || d.TechPriority > 0.5 {
		rules = append(rules, &Rule{
			Name:         "build-advanced-power",
			Priority:     790,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("advanced_power") && PowerExcess() < 50 && Cash() >= 500`,
			Action:       ActionProduceAdvancedPower,
		})
	}

	if d.EconomyPriority > 0.5 {
		siloCap := lerp(0, 2, d.EconomyPriority)
		rules = append(rules, &Rule{
			Name:         "build-ore-silo",
			Priority:     300,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("ore_silo") && ResourcesNearCap() && RoleCount("ore_silo") < %d && Cash() >= 150`, siloCap),
			Action:       ActionProduceOreSilo,
		})
	}

	// --- Production rules (parameterized by unit weights) ---

	if d.InfantryWeight > 0.1 {
		infantryCap := lerp(5, 20, d.InfantryWeight)
		rules = append(rules, &Rule{
			Name:         "produce-infantry",
			Priority:     500,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuild("Infantry","e1") && UnitCount("e1") < %d && Cash() >= 100`, infantryCap),
			Action:       ActionProduceInfantry,
		})
	}

	if d.SpecializedInfantryWeight > 0.1 {
		specialistCap := lerp(1, 6, d.SpecializedInfantryWeight)
		rules = append(rules, &Rule{
			Name:         "produce-specialist-infantry",
			Priority:     490,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuildAnySpecialist() && SpecialistInfantryCount() < %d && Cash() >= 300`, specialistCap),
			Action:       ActionProduceSpecialistInfantry,
		})
	}

	if d.VehicleWeight > 0.1 {
		vehicleCap := lerp(3, 10, d.VehicleWeight)
		rules = append(rules, &Rule{
			Name:         "produce-vehicle",
			Priority:     480,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildAnyCombatVehicle() && CombatVehicleCount() < %d && Cash() >= 800`, vehicleCap),
			Action:       ActionProduceVehicle,
		})
	}

	if d.AirWeight > 0.1 {
		airCap := lerp(2, 8, d.AirWeight)
		rules = append(rules, &Rule{
			Name:         "produce-aircraft",
			Priority:     460,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("airfield") && !QueueBusy("Aircraft") && CanBuildAnyCombatAircraft() && CombatAircraftCount() < %d && Cash() >= 800`, airCap),
			Action:       ActionProduceAircraft,
		})
	}

	if d.NavalWeight > 0.1 {
		navalCap := lerp(2, 6, d.NavalWeight)
		rules = append(rules, &Rule{
			Name:         "produce-ship",
			Priority:     440,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("naval_yard") && !QueueBusy("Ship") && (CanBuildRole("submarine") || CanBuildRole("destroyer")) && (RoleCount("submarine") + RoleCount("destroyer")) < %d && Cash() >= 1000`, navalCap),
			Action:       ActionProduceShip,
		})
	}

	// --- Advanced production rules (gated by TechPriority + unit weights) ---

	if d.InfantryWeight > 0.1 && d.TechPriority > 0.3 {
		rocketCap := lerp(2, 8, d.TechPriority*d.InfantryWeight)
		rules = append(rules, &Rule{
			Name:         "produce-rocket-soldier",
			Priority:     495,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuildRole("rocket_soldier") && RoleCount("rocket_soldier") < %d && Cash() >= 300`, rocketCap),
			Action:       ActionProduceRocketSoldier,
		})
	}

	if d.VehicleWeight > 0.1 && d.TechPriority > 0.3 {
		heavyCap := lerp(1, 5, d.TechPriority*d.VehicleWeight)
		rules = append(rules, &Rule{
			Name:         "produce-heavy-vehicle",
			Priority:     475,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && HasRole("tech_center") && !QueueBusy("Vehicle") && (CanBuildRole("heavy_tank") || CanBuildRole("medium_tank")) && (RoleCount("heavy_tank") + RoleCount("medium_tank")) < %d && Cash() >= 1200`, heavyCap),
			Action:       ActionProduceHeavyVehicle,
		})
	}

	if d.AirWeight > 0.1 && d.TechPriority > 0.4 {
		advAirCap := lerp(1, 4, d.TechPriority*d.AirWeight)
		rules = append(rules, &Rule{
			Name:         "produce-attack-aircraft",
			Priority:     455,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("airfield") && !QueueBusy("Aircraft") && CanBuildRole("advanced_aircraft") && RoleCount("advanced_aircraft") < %d && Cash() >= 1500`, advAirCap),
			Action:       ActionProduceAdvancedAircraft,
		})
	}

	if d.NavalWeight > 0.1 && d.TechPriority > 0.3 {
		advNavalCap := lerp(1, 3, d.TechPriority*d.NavalWeight)
		rules = append(rules, &Rule{
			Name:         "produce-advanced-ship",
			Priority:     435,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("naval_yard") && !QueueBusy("Ship") && (CanBuildRole("cruiser") || CanBuildRole("destroyer")) && (RoleCount("cruiser") + RoleCount("destroyer")) < %d && Cash() >= 2000`, advNavalCap),
			Action:       ActionProduceAdvancedShip,
		})
	}

	// --- Defense rules (parameterized by GroundDefensePriority / AirDefensePriority) ---

	// Defend-base fires when enemies are near buildings. High defense priority
	// means responding with fewer idle units and at higher rule priority.
	defendMinUnits := lerp(3, 1, d.GroundDefensePriority)
	defendPriority := lerp(350, 500, d.GroundDefensePriority)
	rules = append(rules, &Rule{
		Name:         "defend-base",
		Priority:     defendPriority,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`BaseUnderAttack() && len(IdleGroundUnits()) >= %d`, defendMinUnits),
		Action:       ActionDefendBase,
	})

	airDefendPriority := lerp(350, 500, d.AirDefensePriority)
	rules = append(rules, &Rule{
		Name:         "defend-base-air",
		Priority:     airDefendPriority,
		Category:     "air_combat",
		Exclusive:    false,
		ConditionSrc: `BaseUnderAttack() && len(IdleCombatAircraft()) > 0`,
		Action:       ActionAirDefendBase,
	})

	// --- Ground combat rules (parameterized by Aggression + GroundAttackGroupSize) ---

	attackPriority := lerp(200, 400, d.Aggression)
	rules = append(rules, &Rule{
		Name:         "attack-idle-units",
		Priority:     attackPriority,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`len(IdleGroundUnits()) >= %d && NearestEnemy() != nil`, d.GroundAttackGroupSize),
		Action:       ActionAttackMoveIdleGroundUnits,
	})

	// Attack a remembered enemy base when no enemies are currently visible.
	rules = append(rules, &Rule{
		Name:         "attack-known-base",
		Priority:     attackPriority - 10,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`!EnemiesVisible() && HasEnemyIntel() && len(IdleGroundUnits()) >= %d`, d.GroundAttackGroupSize),
		Action:       ActionAttackKnownBaseGround,
	})

	// --- Air combat rules (gated on AirWeight > 0.1) ---

	if d.AirWeight > 0.1 {
		airAttackPriority := lerp(200, 400, d.Aggression) - 5
		rules = append(rules, &Rule{
			Name:         "air-attack-enemy",
			Priority:     airAttackPriority,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`len(IdleCombatAircraft()) >= %d && NearestEnemy() != nil`, d.AirAttackGroupSize),
			Action:       ActionAirAttackEnemy,
		})

		rules = append(rules, &Rule{
			Name:         "air-attack-known-base",
			Priority:     airAttackPriority - 10,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`!EnemiesVisible() && HasEnemyIntel() && len(IdleCombatAircraft()) >= %d`, d.AirAttackGroupSize),
			Action:       ActionAirAttackKnownBase,
		})
	}

	// --- Naval combat rules (gated on NavalWeight > 0.1) ---

	if d.NavalWeight > 0.1 {
		navalAttackPriority := lerp(200, 400, d.Aggression) - 15
		rules = append(rules, &Rule{
			Name:         "naval-attack-enemy",
			Priority:     navalAttackPriority,
			Category:     "naval_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`len(IdleNavalUnits()) >= %d && NearestEnemy() != nil`, d.NavalAttackGroupSize),
			Action:       ActionNavalAttackEnemy,
		})
	}

	// --- Recon rules (parameterized by ScoutPriority) ---
	// Only scout when we haven't located the enemy yet.
	// Requires the ground army to reach attack strength before sending scouts.

	scoutPriority := lerp(250, 400, d.ScoutPriority)
	rules = append(rules, &Rule{
		Name:         "scout-with-idle-units",
		Priority:     scoutPriority,
		Category:     "recon",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`!EnemiesVisible() && !HasEnemyIntel() && len(IdleGroundUnits()) >= %d`, d.GroundAttackGroupSize),
		Action:       ActionScoutWithIdleUnits,
	})

	return rules
}
