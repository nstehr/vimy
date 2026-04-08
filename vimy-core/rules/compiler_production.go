package rules

import (
	"fmt"
	"strings"
)

// addProductionRules emits rules for all unit production: infantry (basic,
// specialist, grenadier, attack dog, spy), vehicles (combat, heavy, scout,
// siege, flak, MAD tank, minelayer), aircraft (basic, advanced), and naval
// (ships, gunboats, advanced). Also includes specialist infantry prerequisite
// buildings and bridge infantry.
func (c *doctrineCompiler) addProductionRules() {
	// --- Unit production ---

	// Boost specialist priority when the LLM's top infantry preference is a
	// specialist role. The preference list is ordered — first entry is most
	// desired. If the LLM wants flamethrowers first, specialists should get
	// first crack at the Infantry queue. If the top preference is a
	// non-specialist (engineer, rocket_soldier) or absent, rifles stay
	// dominant as cheap baseline filler.
	infantryBasePri := 500
	specialistBasePri := 490
	if len(c.d.PreferredInfantry) > 0 {
		specialistRoleSet := make(map[string]bool, len(specialistInfantryRoles))
		for _, r := range specialistInfantryRoles {
			specialistRoleSet[r] = true
		}
		if specialistRoleSet[c.d.PreferredInfantry[0]] {
			specialistBasePri = 500
			infantryBasePri = 490
		}
	}

	// --- Specialist infantry prerequisite buildings ---
	// When the doctrine prefers specialist infantry, ensure the prerequisite
	// buildings are queued. Without these, the specialist units never appear
	// in the Buildable list and production never fires.

	if c.d.SpecializedInfantryWeight > DoctrineEnabled && c.prefersInfantry("flamethrower") {
		// Flamethrower (e4) requires a Flame Tower (ftur) — Defense queue.
		c.rules = append(c.rules, &Rule{
			Name:         "build-flame-tower-for-flamethrower",
			Priority:     555,
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Defense") && CanBuildRole("flame_tower") && !HasRole("flame_tower") && HasRole("barracks") && PowerExcess() >= 0 && Cash() >= 600`,
			Action:       ActionProduceFlameTower,
		})
	}

	if c.d.SpecializedInfantryWeight > DoctrineEnabled && c.prefersInfantry("shock_trooper") {
		// Shock Trooper (shok) requires Soviet Tech Center (stek) + Tesla Coil (tsla).

		// Tech center — only add this rule if TechPriority didn't already
		// include it (DoctrineHigh threshold). This avoids duplicate rules.
		if c.d.TechPriority <= DoctrineHigh {
			c.rules = append(c.rules, &Rule{
				Name:         "build-tech-center-for-shock-trooper",
				Priority:     560,
				Category:     "economy",
				Exclusive:    true,
				ConditionSrc: `!QueueBusy("Building") && CanBuildRole("tech_center") && !HasRole("tech_center") && HasRole("radar") && PowerExcess() >= 0 && Cash() >= 1500`,
				Action:       ActionProduceTechCenter,
			})
		}

		// Tesla Coil — Defense queue prerequisite for shock troopers.
		c.rules = append(c.rules, &Rule{
			Name:         "build-tesla-coil-for-shock-trooper",
			Priority:     555,
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Defense") && CanBuildRole("tesla_coil") && !HasRole("tesla_coil") && HasRole("barracks") && PowerExcess() >= 0 && Cash() >= 800`,
			Action:       ActionProduceTeslaCoil,
		})
	}

	if c.d.SpecializedInfantryWeight > DoctrineEnabled {
		specialistCap := lerp(1, 6, c.d.SpecializedInfantryWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-specialist-infantry",
			Priority:     specialistBasePri,
			Category:     CatProduceInfantry,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuildAnySpecialist() && SpecialistInfantryCount() < %d && %s`, specialistCap, buildCashCondition(300, c.infantrySavings)),
			Action:       ActionProduceSpecialistInfantry,
		})
	}

	if c.d.InfantryWeight > DoctrineEnabled {
		infantryCap := lerp(8, 20, c.d.InfantryWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-infantry",
			Priority:     infantryBasePri,
			Category:     CatProduceInfantry,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuild("Infantry","e1") && UnitCount("e1") < %d && %s`, infantryCap, buildCashCondition(100, c.infantrySavings)),
			Action:       ActionProduceInfantry,
		})

		// Bridge infantry: produce extra e1 while doctrine-desired production
		// buildings are still missing (teching up). Once all buildings come
		// online the rule deactivates and normal caps govern composition.
		var bridgeMissing []string
		bridgeBonus := 0
		if c.d.AirWeight > DoctrineEnabled {
			bridgeMissing = append(bridgeMissing, `!HasRole("airfield")`)
			bridgeBonus += lerp(2, 5, c.d.AirWeight)
		}
		if c.d.NavalWeight > DoctrineEnabled {
			bridgeMissing = append(bridgeMissing, `!HasRole("naval_yard")`)
			bridgeBonus += lerp(1, 4, c.d.NavalWeight)
		}
		if c.d.VehicleWeight > DoctrineModerate {
			bridgeMissing = append(bridgeMissing, `!HasRole("war_factory")`)
			bridgeBonus += lerp(1, 3, c.d.VehicleWeight)
		}
		if len(bridgeMissing) > 0 {
			bridgeCap := infantryCap + bridgeBonus
			missingCond := "(" + strings.Join(bridgeMissing, " || ") + ")"
			c.rules = append(c.rules, &Rule{
				Name:         "produce-bridge-infantry",
				Priority:     infantryBasePri - 5,
				Category:     CatProduceInfantry,
				Exclusive:    true,
				ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuild("Infantry","e1") && %s && UnitCount("e1") >= %d && UnitCount("e1") < %d && %s`, missingCond, infantryCap, bridgeCap, buildCashCondition(100, c.infantrySavings)),
				Action:       ActionProduceInfantry,
			})
		}
	}

	// Grenadier — Soviet anti-structure infantry, complements rifles.
	if c.d.InfantryWeight > DoctrineModerate {
		grenadierCap := lerp(2, 6, c.d.InfantryWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-grenadier",
			Priority:     infantryBasePri - 2,
			Category:     CatProduceInfantry,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuildRole("grenadier") && RoleCount("grenadier") < %d && %s`, grenadierCap, buildCashCondition(160, c.infantrySavings)),
			Action:       ActionProduceGrenadier,
		})
	}

	// Attack dog — cheap, fast scout and anti-spy unit. Requires kennel (Soviet).
	if c.d.InfantryWeight > DoctrineModerate {
		// Build kennel when infantry doctrine warrants it.
		c.rules = append(c.rules, &Rule{
			Name:         "build-kennel",
			Priority:     550,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("kennel") && !HasRole("kennel") && PowerExcess() >= 0 && Cash() >= 300`,
			Action:       ActionProduceKennel,
		})

		dogCap := lerp(1, 3, c.d.InfantryWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-attack-dog",
			Priority:     infantryBasePri - 8,
			Category:     CatProduceInfantry,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("kennel") && !QueueBusy("Infantry") && CanBuildRole("attack_dog") && RoleCount("attack_dog") < %d && %s`, dogCap, buildCashCondition(200, c.infantrySavings)),
			Action:       ActionProduceAttackDog,
		})
	}

	// Spy — Allied infiltration unit. Ties to capture/scout priority.
	if c.d.CapturePriority > DoctrineModerate || c.d.ScoutPriority > DoctrineSignificant {
		c.rules = append(c.rules, &Rule{
			Name:         "produce-spy",
			Priority:     440,
			Category:     CatProduceInfantry,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && HasRole("radar") && !QueueBusy("Infantry") && CanBuildRole("spy") && RoleCount("spy") < 1 && %s`, buildCashCondition(500, c.infantrySavings)),
			Action:       ActionProduceSpy,
		})
	}

	if c.d.VehicleWeight > DoctrineEnabled {
		vehicleCap := lerp(3, 10, c.d.VehicleWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-vehicle",
			Priority:     480,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildAnyCombatVehicle() && CombatVehicleCount() < %d && %s`, vehicleCap, buildCashCondition(800, c.savings)),
			Action:       ActionProduceVehicle,
		})
	}

	if c.d.AirWeight > DoctrineEnabled {
		airCap := lerp(2, 8, c.d.AirWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-aircraft",
			Priority:     460,
			Category:     CatProduceAircraft,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("airfield") && !QueueBusy("Aircraft") && CanBuildAnyCombatAircraft() && CombatAircraftCount() < %d && %s`, airCap, buildCashCondition(800, c.savings)),
			Action:       ActionProduceAircraft,
		})
	}

	if c.d.NavalWeight > DoctrineEnabled {
		navalCap := lerp(3, 8, c.d.NavalWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-ship",
			Priority:     440,
			Category:     CatProduceShip,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && HasRole("naval_yard") && !QueueBusy("Ship") && (CanBuildRole("submarine") || CanBuildRole("destroyer")) && (RoleCount("submarine") + RoleCount("destroyer")) < %d && %s`, navalCap, buildCashCondition(800, c.savings)),
			Action:       ActionProduceShip,
		})
	}

	if c.d.NavalWeight > DoctrineEnabled {
		gunboatCap := lerp(1, 4, c.d.NavalWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-gunboat",
			Priority:     435,
			Category:     CatProduceShip,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && HasRole("naval_yard") && !QueueBusy("Ship") && CanBuildRole("gunboat") && RoleCount("gunboat") < %d && %s`, gunboatCap, buildCashCondition(500, c.savings)),
			Action:       ActionProduceGunboat,
		})
	}

	// --- Advanced unit production (requires tech center) ---

	if c.d.InfantryWeight > DoctrineEnabled && c.d.TechPriority > DoctrineSignificant {
		rocketCap := lerp(2, 8, c.d.TechPriority*c.d.InfantryWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-rocket-soldier",
			Priority:     495,
			Category:     CatProduceInfantry,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuildRole("rocket_soldier") && RoleCount("rocket_soldier") < %d && %s`, rocketCap, buildCashCondition(300, c.infantrySavings)),
			Action:       ActionProduceRocketSoldier,
		})
	}

	if c.d.VehicleWeight > DoctrineEnabled && c.d.TechPriority > DoctrineSignificant {
		heavyCap := lerp(1, 5, c.d.TechPriority*c.d.VehicleWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-heavy-vehicle",
			Priority:     475,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && HasRole("tech_center") && !QueueBusy("Vehicle") && (CanBuildRole("heavy_tank") || CanBuildRole("medium_tank")) && (RoleCount("heavy_tank") + RoleCount("medium_tank")) < %d && %s`, heavyCap, buildCashCondition(1200, c.savings)),
			Action:       ActionProduceHeavyVehicle,
		})
	}

	if c.d.VehicleWeight > DoctrineEnabled {
		c.rules = append(c.rules, &Rule{
			Name:         "produce-scout-vehicle",
			Priority:     465,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!HasEnemyIntel() && HasRole("war_factory") && !QueueBusy("Vehicle") && (CanBuildRole("ranger") || CanBuildRole("light_tank")) && !HasRole("ranger") && !HasScout() && %s`, buildCashCondition(500, c.savings)),
			Action:       ActionProduceScoutVehicle,
		})
	}

	if c.d.VehicleWeight > DoctrineModerate {
		siegeCap := lerp(1, 3, c.d.VehicleWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-siege-vehicle",
			Priority:     460,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && HasRole("radar") && !QueueBusy("Vehicle") && (CanBuildRole("artillery") || CanBuildRole("v2_launcher")) && (RoleCount("artillery") + RoleCount("v2_launcher")) < %d && %s`, siegeCap, buildCashCondition(900, c.savings)),
			Action:       ActionProduceSiegeVehicle,
		})
	}

	if c.d.AirDefensePriority > DoctrineModerate {
		flakCap := lerp(1, 3, c.d.AirDefensePriority)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-flak-truck",
			Priority:     470,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildRole("flak_truck") && RoleCount("flak_truck") < %d && %s`, flakCap, buildCashCondition(600, c.savings)),
			Action:       ActionProduceFlakTruck,
		})
	}

	// MAD tank — Soviet area-denial vehicle. Requires tech center + service depot.
	// High aggression doctrines get more — it's a suicide unit for pushing.
	if c.d.Aggression > DoctrineSignificant && c.d.TechPriority > DoctrineSignificant {
		madCap := lerp(1, 2, c.d.Aggression)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-mad-tank",
			Priority:     455,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && HasRole("tech_center") && !QueueBusy("Vehicle") && CanBuildRole("mad_tank") && RoleCount("mad_tank") < %d && %s`, madCap, buildCashCondition(2000, c.savings)),
			Action:       ActionProduceMADTank,
		})
	}

	// Minelayer — area denial vehicle for defensive doctrines.
	if c.d.GroundDefensePriority > DoctrineSignificant {
		mineCap := lerp(1, 2, c.d.GroundDefensePriority)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-minelayer",
			Priority:     450,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && HasRole("service_depot") && !QueueBusy("Vehicle") && CanBuildRole("minelayer") && RoleCount("minelayer") < %d && %s`, mineCap, buildCashCondition(800, c.savings)),
			Action:       ActionProduceMinelayer,
		})

	}

	// Send idle minelayers to lay mines. Always present so minelayers
	// produced by any path get utilized. With enemy intel, mines go
	// toward the enemy; without, they form a defensive perimeter.
	c.rules = append(c.rules, &Rule{
		Name:         "lay-mines",
		Priority:     300,
		Category:     "minelayer",
		Exclusive:    false,
		ConditionSrc: `len(IdleMinelayers()) > 0`,
		Action:       ActionLayMines,
	})

	if c.d.AirWeight > DoctrineModerate {
		basicAirCap := lerp(1, 3, c.d.AirWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-basic-aircraft",
			Priority:     445,
			Category:     CatProduceAircraft,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("airfield") && !QueueBusy("Aircraft") && CanBuildRole("basic_aircraft") && RoleCount("basic_aircraft") < %d && %s`, basicAirCap, buildCashCondition(1500, c.savings)),
			Action:       ActionProduceBasicAircraft,
		})
	}

	if c.d.AirWeight > DoctrineEnabled && c.d.TechPriority > DoctrineHigh {
		advAirCap := lerp(1, 4, c.d.TechPriority*c.d.AirWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-attack-aircraft",
			Priority:     455,
			Category:     CatProduceAircraft,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("airfield") && !QueueBusy("Aircraft") && CanBuildRole("advanced_aircraft") && RoleCount("advanced_aircraft") < %d && %s`, advAirCap, buildCashCondition(1500, c.savings)),
			Action:       ActionProduceAdvancedAircraft,
		})
	}

	if c.d.NavalWeight > DoctrineEnabled && c.d.TechPriority > DoctrineSignificant {
		advNavalCap := lerp(1, 3, c.d.TechPriority*c.d.NavalWeight)
		c.rules = append(c.rules, &Rule{
			Name:         "produce-advanced-ship",
			Priority:     430,
			Category:     CatProduceShip,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && HasRole("naval_yard") && !QueueBusy("Ship") && (CanBuildRole("cruiser") || CanBuildRole("destroyer") || CanBuildRole("missile_sub")) && (RoleCount("cruiser") + RoleCount("destroyer") + RoleCount("missile_sub")) < %d && %s`, advNavalCap, buildCashCondition(2000, c.savings)),
			Action:       ActionProduceAdvancedShip,
		})
	}
}
