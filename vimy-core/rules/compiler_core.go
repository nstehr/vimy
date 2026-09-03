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
		Category:     "aircraft-maintenance",
		Exclusive:    true,
		ConditionSrc: `QueueReady("Aircraft")`,
		Action:       ActionCancelStuckAircraft,
	})

	// Nudge a unit off the war-factory exit when a vehicle is built but
	// stuck at 100%. Observed live (vimy-zh8): a completed tank couldn't
	// emerge because a parked unit blocked the rally tile, and the whole
	// Vehicle queue hung for the rest of the game.
	c.rules = append(c.rules, &Rule{
		Name:         "unblock-war-factory-egress",
		Priority:     892,
		Category:     "vehicle-maintenance",
		Exclusive:    true,
		ConditionSrc: `HasRole("war_factory") && QueueReady("Vehicle")`,
		Action:       ActionUnblockWarFactoryEgress,
	})

	// Engineer capture sequence: produce engineer → produce APC → load → deliver → capture.
	// The capture-on-foot rule is a fallback for when no APC can be built (no war factory
	// or APC not in buildable list). Without this gate, engineers walk on foot immediately
	// and never wait for the APC.
	// Gated by CapturePriority so pure-defense doctrines don't waste the Infantry queue.

	// Direction rules (capture-building, load, deliver) are emitted whenever
	// engineers or transports already exist, even if CapturePriority is low.
	// Game 16 (vimy-7e1): LLM raised capture_priority briefly to spawn an
	// engineer, then dropped it back to 0 the next eval. Without these
	// outer-gate-free direction rules, the engineer was orphaned with no
	// rule to load/deliver/capture it. Production rules below still gate
	// on CapturePriority so we don't proactively spawn engineers when the
	// doctrine doesn't want them.
	hasCaptureUnitsSrc := `(RoleCount("engineer") > 0 || TransportCount() > 0)`

	c.rules = append(c.rules, &Rule{
		Name:         "capture-building",
		Priority:     850,
		Category:     "capture",
		Exclusive:    false,
		ConditionSrc: `CapturableCount() > 0 && len(IdleEngineers()) > 0 && (!CanBuildTransport() || EngineerNearCapturable())`,
		Action:       ActionCaptureBuilding,
	})

	c.rules = append(c.rules, &Rule{
		Name:      "load-engineer-into-apc",
		Priority:  845,
		Category:  "capture",
		Exclusive: false,
		// Don't re-load an engineer that's already within capture range of a
		// target — otherwise capture-building (priority 850) loses the race
		// to this rule (priority 845) on the tick after unload.
		ConditionSrc: fmt.Sprintf(`%s && len(IdleEngineers()) > 0 && len(IdleEmptyAPCs()) > 0 && !EngineerNearCapturable()`, hasCaptureUnitsSrc),
		Action:       ActionLoadEngineerIntoAPC,
	})

	// Aggressive capture doctrines drop the visibility gate so a loaded
	// APC can dispatch as a scout even before a capturable is revealed.
	// Conservative and orphan-case (capture rules outliving their doctrine)
	// both keep the visibility gate so engineers don't roam aimlessly.
	deliverSrc := `CapturableCount() > 0 && len(IdleEngineerLoadedAPCs()) > 0`
	if c.d.CapturePriority >= DoctrineSignificant {
		deliverSrc = `len(IdleEngineerLoadedAPCs()) > 0`
	}
	c.rules = append(c.rules, &Rule{
		Name:         "deliver-apc-to-target",
		Priority:     847,
		Category:     "capture",
		Exclusive:    false,
		ConditionSrc: deliverSrc,
		Action:       ActionUnloadAPCNearTarget,
	})

	if c.d.CapturePriority > DoctrineEnabled {
		engineerCap := lerp(1, 3, c.d.CapturePriority)

		// Aggressive capture doctrines (CapturePriority >= Significant) build
		// engineers and APCs proactively and use the loaded APC as a scout —
		// rather than waiting for a capturable to be revealed by something else.
		// Conservative capture doctrines keep the "only when we see a target"
		// gates so they don't waste queue slots.
		aggressive := c.d.CapturePriority >= DoctrineSignificant

		produceEngineerSrc := fmt.Sprintf(`CapturableCount() > 0 && !QueueBusy("Infantry") && CanBuildRole("engineer") && RoleCount("engineer") < CapturableCount() && RoleCount("engineer") < %d && Cash() >= 500`, engineerCap)
		produceAPCSrc := `CapturableCount() > 0 && RoleCount("engineer") > 0 && HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildTransport() && TransportCount() < 1 && Cash() >= 800`
		if aggressive {
			produceEngineerSrc = fmt.Sprintf(`!QueueBusy("Infantry") && CanBuildRole("engineer") && RoleCount("engineer") < %d && Cash() >= 500`, engineerCap)
			produceAPCSrc = `RoleCount("engineer") > 0 && HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildTransport() && TransportCount() < 1 && Cash() >= 800`
		}

		c.rules = append(c.rules, &Rule{
			Name:         "produce-engineer",
			Priority:     450,
			Category:     CatProduceInfantry,
			Exclusive:    true,
			ConditionSrc: produceEngineerSrc,
			Action:       ActionProduceEngineer,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "produce-apc",
			Priority:     470,
			Category:     CatProduceVehicle,
			Exclusive:    true,
			ConditionSrc: produceAPCSrc,
			Action:       ActionProduceAPC,
		})

		// Defensive rifle floor for rush doctrines. Engineers can't shoot, so a
		// pure-capture doctrine with InfantryWeight = 0 has nothing to defend
		// the base with while the rush executes. This rule produces up to 3
		// basic rifles at 100 cash each — lowest priority in CatProduceInfantry
		// so produce-engineer (450) always wins when its conditions are met,
		// and produce-infantry (500) supersedes it whenever the doctrine already
		// has real infantry production. It's the floor, not the plan.
		c.rules = append(c.rules, &Rule{
			Name:         "produce-capture-defense-infantry",
			Priority:     440,
			Category:     CatProduceInfantry,
			Exclusive:    true,
			ConditionSrc: `HasRole("barracks") && !QueueBusy("Infantry") && CanBuild("Infantry","e1") && UnitCount("e1") < 3 && Cash() >= 100`,
			Action:       ActionProduceInfantry,
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
		// Priority must exceed produce-vehicle (480) so combat tanks don't
		// monopolize the Vehicle queue and starve APC production.
		assaultAPCPri := lerp(475, 490, c.d.TransportAssault)
		c.rules = append(c.rules, &Rule{
			Name:      "produce-assault-apc",
			Priority:  assaultAPCPri,
			Category:  CatProduceVehicle,
			Exclusive: true,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildTransport() && TransportCount() < %d && %s`,
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

		// Deliver loaded APCs to enemy base. At high TransportAssault we drop
		// the HasEnemyIntel() gate and let the loaded APC scout itself if no
		// building has been sighted yet — otherwise combat-loaded APCs sit at
		// base forever if the enemy attacks with units-only and we never spot
		// their structures. The action handles both branches.
		deliverAssaultSrc := `HasEnemyIntel() && len(IdleCombatLoadedAPCs()) > 0`
		if c.d.TransportAssault >= DoctrineSignificant {
			deliverAssaultSrc = `len(IdleCombatLoadedAPCs()) > 0`
		}
		c.rules = append(c.rules, &Rule{
			Name:         "deliver-assault-apc",
			Priority:     840,
			Category:     "transport",
			Exclusive:    false,
			ConditionSrc: deliverAssaultSrc,
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
		Name:      "scramble-base-defense",
		Priority:  350,
		Category:  "combat",
		Exclusive: false,
		// Condition uses UnassignedIdleGround so we don't fire when only
		// squad members are idle (the action wouldn't do anything anyway
		// after the poaching fix). Emergency-base-defense remains as the
		// last-resort fallback when even squad members must be recalled.
		ConditionSrc: `BaseUnderAttack() && len(UnassignedIdleGround()) > 0`,
		Action:       ActionDefendBase,
	})

	// defend-critical-building: overrides poach-prevention (vimy-d9q) when a
	// critical building is actively taking damage. Pulls ALL near-base ground
	// units regardless of squad membership. Higher priority than scramble
	// and emergency so it wins category dispatch when both would fire.
	c.rules = append(c.rules, &Rule{
		Name:         "defend-critical-building",
		Priority:     360,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: `CriticalBuildingUnderAttack() && len(NearBaseGroundUnits()) > 0`,
		Action:       ActionDefendCriticalBuilding,
	})

	// Emergency recall: when the base is under attack and no idle ground
	// units are available, redirect any nearby ground units (even those with
	// active orders) to defend. This catches units that were given attack-
	// move orders and haven't completed them yet — they appear to be
	// standing at the base but OpenRA considers them "not idle."
	c.rules = append(c.rules, &Rule{
		Name:         "emergency-base-defense",
		Priority:     349,
		Category:     "emergency-defense",
		Exclusive:    false,
		ConditionSrc: `BaseUnderAttack() && len(IdleGroundUnits()) == 0 && len(NearBaseGroundUnits()) > 0`,
		Action:       ActionEmergencyDefendBase,
	})

	c.rules = append(c.rules, &Rule{
		Name:         "scramble-naval-defense",
		Priority:     350,
		Category:     "naval-combat",
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
