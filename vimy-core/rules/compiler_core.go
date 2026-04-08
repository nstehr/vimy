package rules

import "fmt"

// addCoreRules emits rules that are always present regardless of doctrine
// weights: MCV deployment, building placement, engineer capture, transport
// assault, rebuild rules, base defense scramble, repair, and harvester return.
func (c *doctrineCompiler) addCoreRules() {
	// --- Core rules (always present) ---

	c.rules = append(c.rules, &Rule{
		Name:         "deploy-mcv",
		Priority:     1000,
		Category:     "setup",
		Exclusive:    true,
		ConditionSrc: `HasUnit("mcv") && !HasRole("construction_yard")`,
		Action:       ActionDeployMCV,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "recover-mcv",
		Priority:     950,
		Category:     "setup",
		Exclusive:    true,
		ConditionSrc: `!HasRole("construction_yard") && !HasUnit("mcv") && HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuild("Vehicle","mcv") && Cash() >= 1000`,
		Action:       ActionProduceMCV,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "place-ready-building",
		Priority:     900,
		Category:     "economy",
		Exclusive:    true,
		ConditionSrc: `QueueReady("Building")`,
		Action:       ActionPlaceBuilding,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "place-ready-defense",
		Priority:     895,
		Category:     "defense",
		Exclusive:    true,
		ConditionSrc: `QueueReady("Defense")`,
		Action:       ActionPlaceDefense,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "cancel-stuck-aircraft",
		Priority:     891,
		Category:     "aircraft_maintenance",
		Exclusive:    true,
		ConditionSrc: `QueueReady("Aircraft")`,
		Action:       ActionCancelStuckAircraft,
	})

	// Engineer capture sequence: produce engineer → produce APC → load → deliver → capture.
	// The capture-on-foot rule is a fallback for when no APC can be built (no war factory
	// or APC not in buildable list). Without this gate, engineers walk on foot immediately
	// and never wait for the APC.
	// Gated by CapturePriority so pure-defense doctrines don't waste the Infantry queue.

	if c.d.CapturePriority > DoctrineEnabled {
		engineerCap := lerp(1, 3, c.d.CapturePriority)

		c.rules = append(c.rules, &Rule{
			Name:         "capture-building",
			Priority:     850,
			Category:     "capture",
			Exclusive:    false,
			ConditionSrc: `CapturableCount() > 0 && len(IdleEngineers()) > 0 && (!CanBuildRole("apc") || EngineerNearCapturable())`,
			Action:       ActionCaptureBuilding,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "produce-engineer",
			Priority:     450,
			Category:     CatProduceInfantry,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`CapturableCount() > 0 && !QueueBusy("Infantry") && CanBuildRole("engineer") && RoleCount("engineer") < CapturableCount() && RoleCount("engineer") < %d && Cash() >= 500`, engineerCap),
			Action:       ActionProduceEngineer,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "produce-apc",
			Priority:     470,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: `CapturableCount() > 0 && RoleCount("engineer") > 0 && HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildRole("apc") && RoleCount("apc") < 1 && Cash() >= 800`,
			Action:       ActionProduceAPC,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "load-engineer-into-apc",
			Priority:     845,
			Category:     "capture",
			Exclusive:    false,
			ConditionSrc: `len(IdleEngineers()) > 0 && len(IdleEmptyAPCs()) > 0`,
			Action:       ActionLoadEngineerIntoAPC,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "deliver-apc-to-target",
			Priority:     847,
			Category:     "capture",
			Exclusive:    false,
			ConditionSrc: `CapturableCount() > 0 && len(IdleLoadedAPCs()) > 0`,
			Action:       ActionUnloadAPCNearTarget,
		})
	}

	// --- Transport assault ---
	// Loads combat infantry into APCs and rushes them to the enemy base.
	// Relies on infantry production from existing rules (infantry_weight > 0).
	// Capture rules have higher priority (845/847 vs 838/840), so engineers
	// always get APCs first when both workflows are active.

	if c.d.TransportAssault > DoctrineEnabled {
		assaultAPCCap := lerp(1, 3, c.d.TransportAssault)

		// Produce APCs for assault (separate cap from capture APCs).
		c.rules = append(c.rules, &Rule{
			Name:         "produce-assault-apc",
			Priority:     465,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildRole("apc") && RoleCount("apc") < %d && %s`,
				assaultAPCCap, buildCashCondition(800, c.savings)),
			Action: ActionProduceAPC,
		})

		// Load idle combat infantry into empty APCs.
		c.rules = append(c.rules, &Rule{
			Name:         "load-assault-infantry",
			Priority:     838,
			Category:     "transport",
			Exclusive:    false,
			ConditionSrc: `len(IdleCombatInfantry()) > 0 && len(IdleEmptyAPCs()) > 0`,
			Action:       ActionLoadCombatInfantry,
		})

		// Deliver loaded APCs to enemy base.
		c.rules = append(c.rules, &Rule{
			Name:         "deliver-assault-apc",
			Priority:     840,
			Category:     "transport",
			Exclusive:    false,
			ConditionSrc: `HasEnemyIntel() && len(IdleLoadedAPCs()) > 0`,
			Action:       ActionDeliverAssaultAPC,
		})
	}

	// --- Rebuild rules (always present, high priority) ---
	// These fire when a previously-built building is destroyed, using the
	// exclusive "rebuild" category so only one rebuild queues per tick.
	// Harvester rebuild uses the Vehicle queue (not Building), but shares
	// the category so only one rebuild decision is made per tick.

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-power-plant",
		Priority:     840,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("power_plant") && PowerExcess() < 0 && !QueueBusy("Building") && CanBuildRole("power_plant") && Cash() >= 300`,
		Action:       ActionProducePowerPlant,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-advanced-power",
		Priority:     835,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("advanced_power") && PowerExcess() < 0 && !QueueBusy("Building") && CanBuildRole("advanced_power") && Cash() >= 500`,
		Action:       ActionProduceAdvancedPower,
	})

	// Refineries spawn a free harvester, so 1:1 parity is maintained
	// automatically. Only produce a replacement when all harvesters are
	// dead — otherwise the free-spawn timing race causes duplicates
	// (rebuild fires before the free harvester appears in game state).
	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-harvester",
		Priority:     830,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `HasRole("refinery") && RoleCount("harvester") == 0 && !QueueBusy("Vehicle") && CanBuildRole("harvester") && Cash() >= 600`,
		Action:       ActionProduceHarvester,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-refinery",
		Priority:     825,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("refinery") && !QueueBusy("Building") && CanBuildRole("refinery") && Cash() >= 500`,
		Action:       ActionProduceRefinery,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-barracks",
		Priority:     820,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("barracks") && !QueueBusy("Building") && CanBuildRole("barracks") && Cash() >= 200`,
		Action:       ActionProduceBarracks,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-war-factory",
		Priority:     815,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("war_factory") && !QueueBusy("Building") && CanBuildRole("war_factory") && Cash() >= 1000`,
		Action:       ActionProduceWarFactory,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-radar",
		Priority:     810,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("radar") && !QueueBusy("Building") && !QueueProducingRole("radar") && CanBuildRole("radar") && Cash() >= 500`,
		Action:       ActionProduceRadar,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-tech-center",
		Priority:     805,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("tech_center") && !QueueBusy("Building") && CanBuildRole("tech_center") && HasRole("radar") && Cash() >= 1000`,
		Action:       ActionProduceTechCenter,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-airfield",
		Priority:     800,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("airfield") && !QueueBusy("Building") && CanBuildRole("airfield") && Cash() >= 300`,
		Action:       ActionProduceAirfield,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-naval-yard",
		Priority:     800,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `MapHasWater() && LostRole("naval_yard") && !QueueBusy("Building") && CanBuildRole("naval_yard") && Cash() >= 300`,
		Action:       ActionProduceNavalYard,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-service-depot",
		Priority:     795,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("service_depot") && !QueueBusy("Building") && CanBuildRole("service_depot") && Cash() >= 800`,
		Action:       ActionProduceServiceDepot,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-missile-silo",
		Priority:     790,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("missile_silo") && !QueueBusy("Defense") && CanBuildRole("missile_silo") && Cash() >= 2500`,
		Action:       ActionProduceMissileSilo,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-iron-curtain",
		Priority:     785,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("iron_curtain") && !QueueBusy("Defense") && CanBuildRole("iron_curtain") && Cash() >= 2500`,
		Action:       ActionProduceIronCurtain,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "rebuild-kennel",
		Priority:     780,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `LostRole("kennel") && !QueueBusy("Building") && CanBuildRole("kennel") && Cash() >= 200`,
		Action:       ActionProduceKennel,
	})

	// Scramble defense: any idle ground unit responds to a base attack,
	// regardless of squad assignment. The dedicated squad-defend-base and
	// defend-base rules handle their own pools; this catches idle attack-
	// squad members, unassigned units, and any other idle stragglers that
	// would otherwise sit at the base while it's being destroyed.
	c.rules = append(c.rules, &Rule{
		Name:         "scramble-base-defense",
		Priority:     350,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: `BaseUnderAttack() && len(IdleGroundUnits()) > 0`,
		Action:       ActionDefendBase,
	})

	// Emergency recall: when the base is under attack and no idle ground
	// units are available, redirect any nearby ground units (even those with
	// active orders) to defend. This catches units that were given attack-
	// move orders and haven't completed them yet — they appear to be
	// standing at the base but OpenRA considers them "not idle."
	c.rules = append(c.rules, &Rule{
		Name:         "emergency-base-defense",
		Priority:     349,
		Category:     "emergency_defense",
		Exclusive:    false,
		ConditionSrc: `BaseUnderAttack() && len(IdleGroundUnits()) == 0 && len(NearBaseGroundUnits()) > 0`,
		Action:       ActionEmergencyDefendBase,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "scramble-naval-defense",
		Priority:     350,
		Category:     "naval_combat",
		Exclusive:    false,
		ConditionSrc: `MapHasWater() && BaseUnderAttack() && len(IdleNavalUnits()) > 0`,
		Action:       ActionNavalDefendBase,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "repair-buildings",
		Priority:     200,
		Category:     "maintenance",
		Exclusive:    false,
		ConditionSrc: `len(DamagedBuildings()) > 0`,
		Action:       ActionRepairDamagedBuildings,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "return-idle-harvesters",
		Priority:     100,
		Category:     "harvester",
		Exclusive:    false,
		ConditionSrc: `len(IdleHarvesters()) > 0`,
		Action:       ActionSendIdleHarvesters,
	})
}
