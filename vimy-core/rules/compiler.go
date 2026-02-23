package rules

import "fmt"

// Doctrine gate thresholds control which rule blocks CompileDoctrine emits.
// Named constants make the vocabulary self-documenting across 40+ guard checks.
const (
	DoctrineEnabled     = 0.1 // non-trivial weight; include basic rules
	DoctrineModerate    = 0.2 // enough priority to warrant moderate investment
	DoctrineSignificant = 0.3 // warrants dedicated buildings or rule blocks
	DoctrineHigh        = 0.4 // advanced capabilities (tech center, attack aircraft)
	DoctrineDominant    = 0.5 // heavy investment (economy scaling, extra buildings)
	DoctrineExtreme     = 0.6 // extra production buildings for this domain
)

// Priority offsets encode relative rule ordering constraints that aren't
// obvious from bare integer literals.
const (
	SquadFormBonus    = 5  // form-squad fires just above its squad-act rule
	KnownBaseDiscount = 10 // attack-known-base fires below direct-enemy attack
	AirDomainOffset   = 5  // air attack base priority below ground
	NavalDomainOffset = 15 // naval attack base priority below ground
)

// Gameplay thresholds used inside expr condition strings (require fmt.Sprintf).
const (
	LowPowerHeadroom    = 50 // PowerExcess below this triggers advanced power
	IronCurtainMinUnits = 3  // minimum idle ground units to fire iron curtain
)

// buildingSaving prevents unit production from consuming cash needed for
// a high-value building (e.g. tech center at 1500 credits). Once the
// building exists, the savings constraint is released.
type buildingSaving struct {
	existsExpr string // expr condition that the building already exists
	cost       int    // cash needed to queue the building
}

// buildCashCondition generates a cash check that prevents unit production
// from starving out expensive buildings. Without this, a doctrine wanting a
// tech center might never save 1500 credits because infantry keep spending
// at 100 each.
func buildCashCondition(unitCost int, savings []buildingSaving) string {
	cond := fmt.Sprintf("Cash() >= %d", unitCost)
	for _, s := range savings {
		cond += fmt.Sprintf(` && (%s || Cash() >= %d)`, s.existsExpr, unitCost+s.cost)
	}
	return cond
}

// CompileDoctrine translates a Doctrine's continuous 0–1 weights into a
// discrete rule set. Each weight controls which rules are included, their
// relative priorities, and the thresholds in their conditions.
// All expr strings are constructed via fmt.Sprintf — never from user input.
func CompileDoctrine(d Doctrine) []*Rule {
	d.Validate()
	var rules []*Rule

	// Building savings prevent unit spam from starving expensive queued buildings.
	// Added to all production rules' cash conditions below.
	var savings []buildingSaving
	if d.TechPriority > DoctrineSignificant {
		savings = append(savings, buildingSaving{`HasRole("tech_center")`, 1500})
	}
	if d.SuperweaponPriority > DoctrineSignificant {
		savings = append(savings, buildingSaving{
			`HasRole("missile_silo") || HasRole("iron_curtain")`, 2500,
		})
	}

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
		Name:         "cancel-stuck-aircraft",
		Priority:     891,
		Category:     "aircraft_maintenance",
		Exclusive:    true,
		ConditionSrc: `QueueReady("Aircraft")`,
		Action:       ActionCancelStuckAircraft,
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
		Name:         "rebuild-power-plant",
		Priority:     840,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("power_plant") && PowerExcess() < 0 && !QueueBusy("Building") && CanBuildRole("power_plant") && Cash() >= 300`,
		Action:       ActionProducePowerPlant,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-advanced-power",
		Priority:     835,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("advanced_power") && PowerExcess() < 0 && !QueueBusy("Building") && CanBuildRole("advanced_power") && Cash() >= 500`,
		Action:       ActionProduceAdvancedPower,
	})

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
		Name:         "rebuild-missile-silo",
		Priority:     790,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("missile_silo") && !QueueBusy("Defense") && CanBuildRole("missile_silo") && Cash() >= 2500`,
		Action:       ActionProduceMissileSilo,
	})

	rules = append(rules, &Rule{
		Name:         "rebuild-iron-curtain",
		Priority:     785,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("iron_curtain") && !QueueBusy("Defense") && CanBuildRole("iron_curtain") && Cash() >= 2500`,
		Action:       ActionProduceIronCurtain,
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

	// --- Economy ---

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

	// Radar is the tech-tree gate for vehicles, aircraft, and naval —
	// include it whenever any of those paths are desired.
	if d.VehicleWeight > DoctrineEnabled || d.AirWeight > DoctrineEnabled || d.NavalWeight > DoctrineEnabled || d.TechPriority > DoctrineSignificant {
		rules = append(rules, &Rule{
			Name:         "build-radar",
			Priority:     710,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("radar") && !HasRole("radar") && PowerExcess() >= 0 && Cash() >= 1000`,
			Action:       ActionProduceRadar,
		})
	}

	// --- Military buildings ---
	// Priorities scale with weight so the doctrine's emphasis determines
	// build order (e.g. air-heavy → airfield before war factory).

	if d.InfantryWeight > DoctrineEnabled || d.GroundDefensePriority > DoctrineModerate {
		barracksPriority := lerp(600, 700, d.InfantryWeight)
		if d.GroundDefensePriority > DoctrineModerate {
			barracksPriority = max(barracksPriority, lerp(600, 700, d.GroundDefensePriority))
		}
		rules = append(rules, &Rule{
			Name:         "build-barracks",
			Priority:     barracksPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("barracks") && !HasRole("barracks") && PowerExcess() >= 0 && Cash() >= 300`,
			Action:       ActionProduceBarracks,
		})
	}

	if d.VehicleWeight > DoctrineEnabled {
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

	if d.AirWeight > DoctrineEnabled {
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

	if d.VehicleWeight > DoctrineSignificant {
		rules = append(rules, &Rule{
			Name:         "build-service-depot",
			Priority:     570,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("service_depot") && !HasRole("service_depot") && HasRole("war_factory") && PowerExcess() >= 0 && Cash() >= 1200`,
			Action:       ActionProduceServiceDepot,
		})
	}

	if d.NavalWeight > DoctrineEnabled {
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

	// --- Ground defenses ---

	if d.GroundDefensePriority > DoctrineModerate {
		defenseCap := lerp(1, 5, d.GroundDefensePriority)
		defenseCash := lerp(1500, 300, d.GroundDefensePriority)
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

	// --- AA defenses ---

	if d.AirDefensePriority > DoctrineSignificant {
		aaCap := lerp(1, 3, d.AirDefensePriority)
		aaCash := lerp(1200, 500, d.AirDefensePriority)
		aaPriority := lerp(400, 600, d.AirDefensePriority)
		rules = append(rules, &Rule{
			Name:         "build-aa-defense",
			Priority:     aaPriority,
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Defense") && PowerExcess() >= 0 && CanBuildRole("aa_defense") && RoleCount("aa_defense") < %d && Cash() >= %d`, aaCap, aaCash),
			Action:       ActionProduceAADefense,
		})
	}

	// --- Tech progression ---

	if d.TechPriority > DoctrineHigh {
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

	// --- Superweapon buildings ---

	if d.SuperweaponPriority > DoctrineSignificant {
		rules = append(rules, &Rule{
			Name:         "build-missile-silo",
			Priority:     650,
			Category:     "superweapon_build",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Defense") && CanBuildRole("missile_silo") && !HasRole("missile_silo") && HasRole("tech_center") && PowerExcess() >= 0 && Cash() >= 2500`,
			Action:       ActionProduceMissileSilo,
		})

		rules = append(rules, &Rule{
			Name:         "build-iron-curtain",
			Priority:     640,
			Category:     "superweapon_build",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Defense") && CanBuildRole("iron_curtain") && !HasRole("iron_curtain") && HasRole("tech_center") && PowerExcess() >= 0 && Cash() >= 2500`,
			Action:       ActionProduceIronCurtain,
		})
	}

	// --- Extra production buildings ---

	if d.InfantryWeight > DoctrineExtreme {
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

	if d.VehicleWeight > DoctrineExtreme {
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

	if d.AirWeight > DoctrineExtreme {
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

	// --- Economy scaling ---

	if d.EconomyPriority > DoctrineSignificant || d.TechPriority > DoctrineDominant {
		rules = append(rules, &Rule{
			Name:         "build-advanced-power",
			Priority:     790,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("advanced_power") && PowerExcess() < %d && Cash() >= 500`, LowPowerHeadroom),
			Action:       ActionProduceAdvancedPower,
		})
	}

	if d.EconomyPriority > DoctrineDominant {
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

	// --- Unit production ---

	if d.InfantryWeight > DoctrineEnabled {
		infantryCap := lerp(5, 20, d.InfantryWeight)
		rules = append(rules, &Rule{
			Name:         "produce-infantry",
			Priority:     500,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuild("Infantry","e1") && UnitCount("e1") < %d && %s`, infantryCap, buildCashCondition(100, savings)),
			Action:       ActionProduceInfantry,
		})
	}

	if d.SpecializedInfantryWeight > DoctrineEnabled {
		specialistCap := lerp(1, 6, d.SpecializedInfantryWeight)
		rules = append(rules, &Rule{
			Name:         "produce-specialist-infantry",
			Priority:     490,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuildAnySpecialist() && SpecialistInfantryCount() < %d && %s`, specialistCap, buildCashCondition(300, savings)),
			Action:       ActionProduceSpecialistInfantry,
		})
	}

	if d.VehicleWeight > DoctrineEnabled {
		vehicleCap := lerp(3, 10, d.VehicleWeight)
		rules = append(rules, &Rule{
			Name:         "produce-vehicle",
			Priority:     480,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildAnyCombatVehicle() && CombatVehicleCount() < %d && %s`, vehicleCap, buildCashCondition(800, savings)),
			Action:       ActionProduceVehicle,
		})
	}

	if d.AirWeight > DoctrineEnabled {
		airCap := lerp(2, 8, d.AirWeight)
		rules = append(rules, &Rule{
			Name:         "produce-aircraft",
			Priority:     460,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("airfield") && !QueueBusy("Aircraft") && CanBuildAnyCombatAircraft() && CombatAircraftCount() < %d && %s`, airCap, buildCashCondition(800, savings)),
			Action:       ActionProduceAircraft,
		})
	}

	if d.NavalWeight > DoctrineEnabled {
		navalCap := lerp(2, 6, d.NavalWeight)
		rules = append(rules, &Rule{
			Name:         "produce-ship",
			Priority:     440,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("naval_yard") && !QueueBusy("Ship") && (CanBuildRole("submarine") || CanBuildRole("destroyer")) && (RoleCount("submarine") + RoleCount("destroyer")) < %d && %s`, navalCap, buildCashCondition(1000, savings)),
			Action:       ActionProduceShip,
		})
	}

	// --- Advanced unit production (requires tech center) ---

	if d.InfantryWeight > DoctrineEnabled && d.TechPriority > DoctrineSignificant {
		rocketCap := lerp(2, 8, d.TechPriority*d.InfantryWeight)
		rules = append(rules, &Rule{
			Name:         "produce-rocket-soldier",
			Priority:     495,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuildRole("rocket_soldier") && RoleCount("rocket_soldier") < %d && %s`, rocketCap, buildCashCondition(300, savings)),
			Action:       ActionProduceRocketSoldier,
		})
	}

	if d.VehicleWeight > DoctrineEnabled && d.TechPriority > DoctrineSignificant {
		heavyCap := lerp(1, 5, d.TechPriority*d.VehicleWeight)
		rules = append(rules, &Rule{
			Name:         "produce-heavy-vehicle",
			Priority:     475,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && HasRole("tech_center") && !QueueBusy("Vehicle") && (CanBuildRole("heavy_tank") || CanBuildRole("medium_tank")) && (RoleCount("heavy_tank") + RoleCount("medium_tank")) < %d && %s`, heavyCap, buildCashCondition(1200, savings)),
			Action:       ActionProduceHeavyVehicle,
		})
	}

	if d.AirWeight > DoctrineEnabled && d.TechPriority > DoctrineHigh {
		advAirCap := lerp(1, 4, d.TechPriority*d.AirWeight)
		rules = append(rules, &Rule{
			Name:         "produce-attack-aircraft",
			Priority:     455,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("airfield") && !QueueBusy("Aircraft") && CanBuildRole("advanced_aircraft") && RoleCount("advanced_aircraft") < %d && %s`, advAirCap, buildCashCondition(1500, savings)),
			Action:       ActionProduceAdvancedAircraft,
		})
	}

	if d.NavalWeight > DoctrineEnabled && d.TechPriority > DoctrineSignificant {
		advNavalCap := lerp(1, 3, d.TechPriority*d.NavalWeight)
		rules = append(rules, &Rule{
			Name:         "produce-advanced-ship",
			Priority:     435,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("naval_yard") && !QueueBusy("Ship") && (CanBuildRole("cruiser") || CanBuildRole("destroyer")) && (RoleCount("cruiser") + RoleCount("destroyer")) < %d && %s`, advNavalCap, buildCashCondition(2000, savings)),
			Action:       ActionProduceAdvancedShip,
		})
	}

	// --- Defense behavior ---

	defendPriority := lerp(350, 500, d.GroundDefensePriority)

	// High defense priority: reserve a persistent squad so defenders aren't
	// poached by attack rules between engagements.
	if d.GroundDefensePriority > DoctrineSignificant {
		defenseSize := lerp(2, 5, d.GroundDefensePriority)
		rules = append(rules, &Rule{
			Name:         "form-defense-squad",
			Priority:     defendPriority + SquadFormBonus,
			Category:     "squad_form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`!SquadExists("ground-defense") && len(UnassignedIdleGround()) >= %d`, defenseSize),
			Action:       FormSquad("ground-defense", "ground", defenseSize, "defend"),
		})

		rules = append(rules, &Rule{
			Name:         "squad-defend-base",
			Priority:     defendPriority,
			Category:     "combat",
			Exclusive:    false,
			ConditionSrc: `SquadExists("ground-defense") && SquadIdleCount("ground-defense") > 0 && BaseUnderAttack()`,
			Action:       SquadDefend("ground-defense"),
		})
	} else {
		// Low defense: no reserved squad, just scramble all idle ground units.
		defendMinUnits := lerp(3, 1, d.GroundDefensePriority)
		rules = append(rules, &Rule{
			Name:         "defend-base",
			Priority:     defendPriority,
			Category:     "combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`BaseUnderAttack() && len(IdleGroundUnits()) >= %d`, defendMinUnits),
			Action:       ActionDefendBase,
		})
	}

	airDefendPriority := lerp(350, 500, d.AirDefensePriority)
	rules = append(rules, &Rule{
		Name:         "defend-base-air",
		Priority:     airDefendPriority,
		Category:     "air_combat",
		Exclusive:    false,
		ConditionSrc: `BaseUnderAttack() && len(IdleCombatAircraft()) > 0`,
		Action:       ActionAirDefendBase,
	})

	// --- Ground attack ---

	attackPriority := lerp(200, 400, d.Aggression)

	rules = append(rules, &Rule{
		Name:         "form-ground-attack",
		Priority:     attackPriority + SquadFormBonus,
		Category:     "squad_form",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`!SquadExists("ground-attack") && len(UnassignedIdleGround()) >= %d`, d.GroundAttackGroupSize),
		Action:       FormSquad("ground-attack", "ground", d.GroundAttackGroupSize, "attack"),
	})

	rules = append(rules, &Rule{
		Name:         "squad-attack",
		Priority:     attackPriority,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: `SquadExists("ground-attack") && SquadIdleCount("ground-attack") >= SquadSize("ground-attack") && NearestEnemy() != nil`,
		Action:       SquadAttackMove("ground-attack"),
	})

	// Fallback: attack last-known enemy base when fog of war hides all enemies.
	rules = append(rules, &Rule{
		Name:         "squad-attack-known-base",
		Priority:     attackPriority - KnownBaseDiscount,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: `SquadExists("ground-attack") && SquadIdleCount("ground-attack") >= SquadSize("ground-attack") && !EnemiesVisible() && HasEnemyIntel()`,
		Action:       SquadAttackKnownBase("ground-attack"),
	})

	// --- Air attack ---

	if d.AirWeight > DoctrineEnabled {
		airAttackPriority := lerp(200, 400, d.Aggression) - AirDomainOffset

		rules = append(rules, &Rule{
			Name:         "form-air-attack",
			Priority:     airAttackPriority + SquadFormBonus,
			Category:     "squad_form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`!SquadExists("air-attack") && len(UnassignedIdleAir()) >= %d`, d.AirAttackGroupSize),
			Action:       FormSquad("air-attack", "air", d.AirAttackGroupSize, "attack"),
		})

		rules = append(rules, &Rule{
			Name:         "squad-air-attack",
			Priority:     airAttackPriority,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: `SquadExists("air-attack") && SquadIdleCount("air-attack") >= SquadSize("air-attack") && NearestEnemy() != nil`,
			Action:       SquadAttackMove("air-attack"),
		})

		rules = append(rules, &Rule{
			Name:         "squad-air-attack-known-base",
			Priority:     airAttackPriority - KnownBaseDiscount,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: `SquadExists("air-attack") && SquadIdleCount("air-attack") >= SquadSize("air-attack") && !EnemiesVisible() && HasEnemyIntel()`,
			Action:       SquadAttackKnownBase("air-attack"),
		})
	}

	// --- Naval attack ---

	if d.NavalWeight > DoctrineEnabled {
		navalAttackPriority := lerp(200, 400, d.Aggression) - NavalDomainOffset

		rules = append(rules, &Rule{
			Name:         "form-naval-attack",
			Priority:     navalAttackPriority + SquadFormBonus,
			Category:     "squad_form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`!SquadExists("naval-attack") && len(UnassignedIdleNaval()) >= %d`, d.NavalAttackGroupSize),
			Action:       FormSquad("naval-attack", "naval", d.NavalAttackGroupSize, "attack"),
		})

		rules = append(rules, &Rule{
			Name:         "squad-naval-attack",
			Priority:     navalAttackPriority,
			Category:     "naval_combat",
			Exclusive:    false,
			ConditionSrc: `SquadExists("naval-attack") && SquadIdleCount("naval-attack") >= SquadSize("naval-attack") && NearestEnemy() != nil`,
			Action:       SquadAttackMove("naval-attack"),
		})
	}

	// --- Superweapon fire ---

	if d.SuperweaponPriority > DoctrineEnabled {
		rules = append(rules, &Rule{
			Name:         "fire-nuke",
			Priority:     880,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("NukePowerInfoOrder")`,
			Action:       ActionFireNuke,
		})

		rules = append(rules, &Rule{
			Name:         "fire-iron-curtain",
			Priority:     870,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`SupportPowerReady("GrantExternalConditionPowerInfoOrder") && len(IdleGroundUnits()) >= %d`, IronCurtainMinUnits),
			Action:       ActionFireIronCurtain,
		})
	}

	// --- Airfield support powers ---

	if d.AirWeight > DoctrineEnabled {
		rules = append(rules, &Rule{
			Name:         "fire-spy-plane",
			Priority:     860,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("SovietSpyPlane") && !HasEnemyIntel()`,
			Action:       ActionFireSpyPlane,
		})

		rules = append(rules, &Rule{
			Name:         "fire-spy-plane-update",
			Priority:     250,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("SovietSpyPlane") && HasEnemyIntel() && !EnemiesVisible()`,
			Action:       ActionFireSpyPlane,
		})

		rules = append(rules, &Rule{
			Name:         "fire-paratroopers",
			Priority:     855,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("SovietParatroopers") && (HasEnemyIntel() || EnemiesVisible())`,
			Action:       ActionFireParatroopers,
		})

		rules = append(rules, &Rule{
			Name:         "fire-parabombs",
			Priority:     845,
			Category:     "superweapon",
			Exclusive:    true,
			ConditionSrc: `SupportPowerReady("UkraineParabombs") && (HasEnemyIntel() || EnemiesVisible())`,
			Action:       ActionFireParabombs,
		})
	}

	// --- Recon ---
	// Scouts are only sent when the enemy hasn't been located yet, and only
	// after reaching attack strength — so scouting doesn't weaken the army.

	scoutPriority := lerp(250, 400, d.ScoutPriority)
	rules = append(rules, &Rule{
		Name:         "scout-with-idle-units",
		Priority:     scoutPriority,
		Category:     "recon",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`!EnemiesVisible() && !HasEnemyIntel() && len(UnassignedIdleGround()) >= %d`, d.GroundAttackGroupSize),
		Action:       ActionScoutWithIdleUnits,
	})

	return rules
}
