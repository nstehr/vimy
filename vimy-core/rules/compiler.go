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
	ReengageDiscount  = 2  // re-engage fires just below coordinated attack
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
	//
	// Each savings clause is gated on having the prerequisite building so that
	// the reserve doesn't block unit production before the expensive building
	// can actually be queued. Without these gates, moderate tech/superweapon
	// priorities (0.3-0.5) create enormous cash thresholds (2300+ for a tank)
	// that prevent any army from being built in early/mid game.
	var savings []buildingSaving
	if d.TechPriority > DoctrineHigh {
		// Tech center requires radar. Don't reserve 1500 until radar exists.
		// Threshold matches build-tech-center rule (DoctrineHigh) so we never
		// reserve cash for a tech center the doctrine won't actually build.
		savings = append(savings, buildingSaving{`HasRole("tech_center") || !HasRole("radar")`, 1500})
	}
	if d.SuperweaponPriority > DoctrineHigh {
		// Superweapons require tech center. Don't reserve 2500 until it exists.
		savings = append(savings, buildingSaving{
			`HasRole("missile_silo") || HasRole("iron_curtain") || !HasRole("tech_center")`, 2500,
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

	// Engineer capture sequence: produce engineer → produce APC → load → deliver → capture.
	// The capture-on-foot rule is a fallback for when no APC can be built (no war factory
	// or APC not in buildable list). Without this gate, engineers walk on foot immediately
	// and never wait for the APC.
	// Gated by CapturePriority so pure-defense doctrines don't waste the Infantry queue.

	if d.CapturePriority > DoctrineEnabled {
		engineerCap := lerp(1, 3, d.CapturePriority)

		rules = append(rules, &Rule{
			Name:         "capture-building",
			Priority:     850,
			Category:     "capture",
			Exclusive:    false,
			ConditionSrc: `CapturableCount() > 0 && len(IdleEngineers()) > 0 && !CanBuildRole("apc")`,
			Action:       ActionCaptureBuilding,
		})

		rules = append(rules, &Rule{
			Name:         "produce-engineer",
			Priority:     450,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`CapturableCount() > 0 && !QueueBusy("Infantry") && CanBuildRole("engineer") && RoleCount("engineer") < CapturableCount() && RoleCount("engineer") < %d && Cash() >= 500`, engineerCap),
			Action:       ActionProduceEngineer,
		})

		rules = append(rules, &Rule{
			Name:         "produce-apc",
			Priority:     470,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: `CapturableCount() > 0 && RoleCount("engineer") > 0 && HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildRole("apc") && RoleCount("apc") < 1 && Cash() >= 800`,
			Action:       ActionProduceAPC,
		})

		rules = append(rules, &Rule{
			Name:         "load-engineer-into-apc",
			Priority:     845,
			Category:     "capture",
			Exclusive:    false,
			ConditionSrc: `len(IdleEngineers()) > 0 && len(IdleEmptyAPCs()) > 0`,
			Action:       ActionLoadEngineerIntoAPC,
		})

		rules = append(rules, &Rule{
			Name:         "deliver-apc-to-target",
			Priority:     847,
			Category:     "capture",
			Exclusive:    false,
			ConditionSrc: `CapturableCount() > 0 && len(IdleLoadedAPCs()) > 0`,
			Action:       ActionUnloadAPCNearTarget,
		})
	}

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

	// Refineries spawn a free harvester, so 1:1 parity is maintained
	// automatically. Only produce a replacement when all harvesters are
	// dead — otherwise the free-spawn timing race causes duplicates
	// (rebuild fires before the free harvester appears in game state).
	rules = append(rules, &Rule{
		Name:         "rebuild-harvester",
		Priority:     830,
		Category:     "rebuild",
		Exclusive:    true,
		ConditionSrc: `HasRole("refinery") && RoleCount("harvester") == 0 && !QueueBusy("Vehicle") && CanBuildRole("harvester") && Cash() >= 600`,
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
		ConditionSrc: `MapHasWater() && LostRole("naval_yard") && !QueueBusy("Building") && CanBuildRole("naval_yard") && Cash() >= 300`,
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

	// Scramble defense: any idle ground unit responds to a base attack,
	// regardless of squad assignment. The dedicated squad-defend-base and
	// defend-base rules handle their own pools; this catches idle attack-
	// squad members, unassigned units, and any other idle stragglers that
	// would otherwise sit at the base while it's being destroyed.
	rules = append(rules, &Rule{
		Name:         "scramble-base-defense",
		Priority:     350,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: `BaseUnderAttack() && len(IdleGroundUnits()) > 0`,
		Action:       ActionDefendBase,
	})

	rules = append(rules, &Rule{
		Name:         "scramble-naval-defense",
		Priority:     350,
		Category:     "naval_combat",
		Exclusive:    false,
		ConditionSrc: `MapHasWater() && BaseUnderAttack() && len(IdleNavalUnits()) > 0`,
		Action:       ActionNavalDefendBase,
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

	// Initial refineries (1st and 2nd) — critical economy, high priority.
	// Cap at refineryMax so low-economy doctrines don't over-build.
	initialRefCap := min(2, refineryMax)
	rules = append(rules, &Rule{
		Name:         "build-refinery",
		Priority:     750,
		Category:     "economy",
		Exclusive:    true,
		ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("refinery") && RoleCount("refinery") < %d && Cash() >= %d`, initialRefCap, refineryCashThreshold),
		Action:       ActionProduceRefinery,
	})

	// Expansion refineries (3rd+) — economy optimization, below tech progression.
	if d.EconomyPriority > DoctrineSignificant {
		rules = append(rules, &Rule{
			Name:         "build-extra-refinery",
			Priority:     520,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Building") && CanBuildRole("refinery") && RoleCount("refinery") >= 2 && RoleCount("refinery") < %d && (HasRole("barracks") || HasRole("war_factory")) && Cash() >= %d`, refineryMax, refineryCashThreshold),
			Action:       ActionProduceRefinery,
		})
	}

	// --- Prerequisite buildings ---

	// Radar is the tech-tree gate for vehicles, aircraft, and naval —
	// include it whenever any of those paths are desired. Requires at
	// least one military building so it doesn't jump ahead of barracks.
	if d.VehicleWeight > DoctrineEnabled || d.AirWeight > DoctrineEnabled || d.NavalWeight > DoctrineEnabled || d.TechPriority > DoctrineSignificant {
		rules = append(rules, &Rule{
			Name:         "build-radar",
			Priority:     710,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("radar") && !HasRole("radar") && (HasRole("barracks") || HasRole("war_factory")) && PowerExcess() >= 0 && Cash() >= 1000`,
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

	if d.InfantryWeight > DoctrineEnabled || d.GroundDefensePriority > DoctrineModerate {
		barracksIncluded = true
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

	// If radar is included but no military building rule exists yet,
	// add barracks as a prerequisite — it's the cheapest tech-tree gate
	// to radar in RA and prevents a deadlock where radar can never be built.
	radarIncluded := d.VehicleWeight > DoctrineEnabled || d.AirWeight > DoctrineEnabled || d.NavalWeight > DoctrineEnabled || d.TechPriority > DoctrineSignificant
	if radarIncluded && !barracksIncluded && d.VehicleWeight <= DoctrineEnabled {
		rules = append(rules, &Rule{
			Name:         "build-barracks-prereq",
			Priority:     600,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!QueueBusy("Building") && CanBuildRole("barracks") && !HasRole("barracks") && PowerExcess() >= 0 && Cash() >= 300`,
			Action:       ActionProduceBarracks,
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
			ConditionSrc: `MapHasWater() && !QueueBusy("Building") && CanBuildRole("naval_yard") && !HasRole("naval_yard") && PowerExcess() >= 0 && Cash() >= 500`,
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
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Defense") && PowerExcess() >= 0 && (CanBuildRole("pillbox") || CanBuildRole("camo_pillbox") || CanBuildRole("turret") || CanBuildRole("flame_tower") || CanBuildRole("tesla_coil")) && (RoleCount("pillbox") + RoleCount("camo_pillbox") + RoleCount("turret") + RoleCount("flame_tower") + RoleCount("tesla_coil")) < %d && Cash() >= %d`, defenseCap, defenseCash),
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

	// --- Gap generator (Allied strategic defense) ---

	if d.GroundDefensePriority > DoctrineSignificant && d.TechPriority > DoctrineSignificant {
		gapCap := lerp(1, 2, d.GroundDefensePriority)
		rules = append(rules, &Rule{
			Name:         "build-gap-generator",
			Priority:     lerp(400, 550, d.GroundDefensePriority),
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`!QueueBusy("Defense") && PowerExcess() >= 0 && CanBuildRole("gap_generator") && HasRole("tech_center") && RoleCount("gap_generator") < %d && Cash() >= 800`, gapCap),
			Action:       ActionProduceGapGenerator,
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

	if d.NavalWeight > DoctrineExtreme {
		extraNavalCap := lerp(1, 2, d.NavalWeight)
		rules = append(rules, &Rule{
			Name:         "build-extra-naval-yard",
			Priority:     470,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && !QueueBusy("Building") && CanBuildRole("naval_yard") && RoleCount("naval_yard") < %d && PowerExcess() >= 0 && Cash() >= 500`, extraNavalCap),
			Action:       ActionProduceNavalYard,
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

	if d.EconomyPriority > DoctrineDominant {
		rules = append(rules, &Rule{
			Name:         "produce-extra-harvester",
			Priority:     420,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("refinery") && HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildRole("harvester") && RoleCount("harvester") < RoleCount("refinery") + 1 && %s`, buildCashCondition(1400, savings)),
			Action:       ActionProduceHarvester,
		})
	}

	// --- Unit production ---

	// Vehicle-cost reservation: when the doctrine wants both infantry and
	// vehicles, infantry rules save 800 cash headroom so vehicle production
	// can start. The reservation releases once vehicles reach their cap.
	infantrySavings := append([]buildingSaving(nil), savings...)
	if d.VehicleWeight > DoctrineModerate {
		vehicleCapForSaving := lerp(3, 10, d.VehicleWeight)
		infantrySavings = append(infantrySavings, buildingSaving{
			existsExpr: fmt.Sprintf("CombatVehicleCount() >= %d", vehicleCapForSaving),
			cost:       800,
		})
	}

	if d.InfantryWeight > DoctrineEnabled {
		infantryCap := lerp(5, 20, d.InfantryWeight)
		rules = append(rules, &Rule{
			Name:         "produce-infantry",
			Priority:     500,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuild("Infantry","e1") && UnitCount("e1") < %d && %s`, infantryCap, buildCashCondition(100, infantrySavings)),
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
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuildAnySpecialist() && SpecialistInfantryCount() < %d && %s`, specialistCap, buildCashCondition(300, infantrySavings)),
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
		navalCap := lerp(3, 8, d.NavalWeight)
		rules = append(rules, &Rule{
			Name:         "produce-ship",
			Priority:     440,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && HasRole("naval_yard") && !QueueBusy("Ship") && (CanBuildRole("submarine") || CanBuildRole("destroyer")) && (RoleCount("submarine") + RoleCount("destroyer")) < %d && %s`, navalCap, buildCashCondition(800, savings)),
			Action:       ActionProduceShip,
		})
	}

	if d.NavalWeight > DoctrineEnabled {
		gunboatCap := lerp(1, 4, d.NavalWeight)
		rules = append(rules, &Rule{
			Name:         "produce-gunboat",
			Priority:     435,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && HasRole("naval_yard") && !QueueBusy("Ship") && CanBuildRole("gunboat") && RoleCount("gunboat") < %d && %s`, gunboatCap, buildCashCondition(500, savings)),
			Action:       ActionProduceGunboat,
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
			ConditionSrc: fmt.Sprintf(`HasRole("barracks") && !QueueBusy("Infantry") && CanBuildRole("rocket_soldier") && RoleCount("rocket_soldier") < %d && %s`, rocketCap, buildCashCondition(300, infantrySavings)),
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

	if d.VehicleWeight > DoctrineEnabled {
		rules = append(rules, &Rule{
			Name:         "produce-scout-vehicle",
			Priority:     465,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`!HasEnemyIntel() && HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildRole("ranger") && RoleCount("ranger") < 1 && %s`, buildCashCondition(500, savings)),
			Action:       ActionProduceScoutVehicle,
		})
	}

	if d.VehicleWeight > DoctrineModerate {
		siegeCap := lerp(1, 3, d.VehicleWeight)
		rules = append(rules, &Rule{
			Name:         "produce-siege-vehicle",
			Priority:     460,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && HasRole("radar") && !QueueBusy("Vehicle") && (CanBuildRole("artillery") || CanBuildRole("v2_launcher")) && (RoleCount("artillery") + RoleCount("v2_launcher")) < %d && %s`, siegeCap, buildCashCondition(900, savings)),
			Action:       ActionProduceSiegeVehicle,
		})
	}

	if d.AirDefensePriority > DoctrineModerate {
		flakCap := lerp(1, 3, d.AirDefensePriority)
		rules = append(rules, &Rule{
			Name:         "produce-flak-truck",
			Priority:     470,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("war_factory") && !QueueBusy("Vehicle") && CanBuildRole("flak_truck") && RoleCount("flak_truck") < %d && %s`, flakCap, buildCashCondition(600, savings)),
			Action:       ActionProduceFlakTruck,
		})
	}

	if d.AirWeight > DoctrineModerate {
		basicAirCap := lerp(1, 3, d.AirWeight)
		rules = append(rules, &Rule{
			Name:         "produce-basic-aircraft",
			Priority:     445,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`HasRole("airfield") && !QueueBusy("Aircraft") && CanBuildRole("basic_aircraft") && RoleCount("basic_aircraft") < %d && %s`, basicAirCap, buildCashCondition(1500, savings)),
			Action:       ActionProduceBasicAircraft,
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
			Priority:     430,
			Category:     "production",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && HasRole("naval_yard") && !QueueBusy("Ship") && (CanBuildRole("cruiser") || CanBuildRole("destroyer")) && (RoleCount("cruiser") + RoleCount("destroyer")) < %d && %s`, advNavalCap, buildCashCondition(2000, savings)),
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
			ConditionSrc: fmt.Sprintf(`(!SquadExists("ground-defense") && len(UnassignedIdleGround()) >= %d) || (SquadNeedsReinforcement("ground-defense") && len(UnassignedIdleGround()) >= 1)`, defenseSize),
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
	activationThreshold := lerpf(0.6, 1.0, 1.0-d.Aggression)

	rules = append(rules, &Rule{
		Name:         "form-ground-attack",
		Priority:     attackPriority + SquadFormBonus,
		Category:     "squad_form",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`(!SquadExists("ground-attack") && len(UnassignedIdleGround()) >= %d) || (SquadNeedsReinforcement("ground-attack") && len(UnassignedIdleGround()) >= 1)`, d.GroundAttackGroupSize),
		Action:       FormSquad("ground-attack", "ground", d.GroundAttackGroupSize, "attack"),
	})

	rules = append(rules, &Rule{
		Name:         "squad-attack",
		Priority:     attackPriority,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`SquadExists("ground-attack") && SquadReadyRatio("ground-attack") >= %.2f && (BestGroundTarget() != nil || NearestEnemy() != nil)`, activationThreshold),
		Action:       SquadAttackMove("ground-attack"),
	})

	// Re-engage idle squad members already in the field — no ratio gate.
	// The main squad-attack rule handles the initial coordinated launch;
	// this handles the common case where some members finish their order
	// while others are still fighting.
	rules = append(rules, &Rule{
		Name:         "squad-reengage",
		Priority:     attackPriority - ReengageDiscount,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: `SquadExists("ground-attack") && SquadIdleCount("ground-attack") > 0 && (BestGroundTarget() != nil || NearestEnemy() != nil)`,
		Action:       SquadAttackMove("ground-attack"),
	})

	// Fallback: attack last-known enemy base when fog of war hides all enemies.
	rules = append(rules, &Rule{
		Name:         "squad-attack-known-base",
		Priority:     attackPriority - KnownBaseDiscount,
		Category:     "combat",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`SquadExists("ground-attack") && SquadReadyRatio("ground-attack") >= %.2f && !EnemiesVisible() && HasEnemyIntel()`, activationThreshold),
		Action:       SquadAttackKnownBase("ground-attack", d.Aggression),
	})

	// --- Air attack ---

	if d.AirWeight > DoctrineEnabled {
		airAttackPriority := lerp(200, 400, d.Aggression) - AirDomainOffset

		rules = append(rules, &Rule{
			Name:         "form-air-attack",
			Priority:     airAttackPriority + SquadFormBonus,
			Category:     "squad_form",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`(!SquadExists("air-attack") && len(UnassignedIdleAir()) >= %d) || (SquadNeedsReinforcement("air-attack") && len(UnassignedIdleAir()) >= 1)`, d.AirAttackGroupSize),
			Action:       FormSquad("air-attack", "air", d.AirAttackGroupSize, "attack"),
		})

		rules = append(rules, &Rule{
			Name:         "squad-air-attack",
			Priority:     airAttackPriority,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("air-attack") && SquadReadyRatio("air-attack") >= %.2f && BestAirTarget() != nil`, activationThreshold),
			Action:       SquadAirStrike("air-attack"),
		})

		rules = append(rules, &Rule{
			Name:         "squad-air-reengage",
			Priority:     airAttackPriority - ReengageDiscount,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: `SquadExists("air-attack") && SquadIdleCount("air-attack") > 0 && BestAirTarget() != nil`,
			Action:       SquadAirStrike("air-attack"),
		})

		rules = append(rules, &Rule{
			Name:         "squad-air-attack-known-base",
			Priority:     airAttackPriority - KnownBaseDiscount,
			Category:     "air_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("air-attack") && SquadReadyRatio("air-attack") >= %.2f && !EnemiesVisible() && HasEnemyIntel()`, activationThreshold),
			Action:       SquadAttackKnownBase("air-attack", d.Aggression),
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
			ConditionSrc: fmt.Sprintf(`MapHasWater() && ((!SquadExists("naval-attack") && len(UnassignedIdleNaval()) >= %d) || (SquadNeedsReinforcement("naval-attack") && len(UnassignedIdleNaval()) >= 1))`, d.NavalAttackGroupSize),
			Action:       FormSquad("naval-attack", "naval", d.NavalAttackGroupSize, "attack"),
		})

		rules = append(rules, &Rule{
			Name:         "squad-naval-attack",
			Priority:     navalAttackPriority,
			Category:     "naval_combat",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`MapHasWater() && SquadExists("naval-attack") && SquadReadyRatio("naval-attack") >= %.2f && NearestEnemy() != nil`, activationThreshold),
			Action:       SquadAttackMove("naval-attack"),
		})

		rules = append(rules, &Rule{
			Name:         "squad-naval-reengage",
			Priority:     navalAttackPriority - ReengageDiscount,
			Category:     "naval_combat",
			Exclusive:    false,
			ConditionSrc: `MapHasWater() && SquadExists("naval-attack") && SquadIdleCount("naval-attack") > 0 && NearestEnemy() != nil`,
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

	// --- Micro behaviors ---
	// Category "micro" is non-exclusive so all micro rules can co-fire on the same tick.

	// Retreat damaged combat units — always present since all doctrines have combat units.
	retreatThreshold := lerpf(0.50, 0.15, d.Aggression)
	retreatPriority := lerp(380, 450, 1.0-d.Aggression)
	rules = append(rules, &Rule{
		Name:         "retreat-damaged-units",
		Priority:     retreatPriority,
		Category:     "micro",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`len(DamagedCombatUnits(%.2f)) > 0`, retreatThreshold),
		Action:       RetreatDamagedUnits(retreatThreshold),
	})

	// Clear healed units from retreating set — runs every tick so healed
	// units return to the combat pool promptly.
	rules = append(rules, &Rule{
		Name:         "clear-healed-units",
		Priority:     500,
		Category:     "micro",
		Exclusive:    false,
		ConditionSrc: "HasRetreatingUnits()",
		Action:       ClearHealedUnits(retreatThreshold),
	})

	// Chase leash — recall overextended squad members that wandered off after kills.
	// Leash distance scales with aggression — aggressive doctrines let units roam further.
	leashPct := lerpf(0.25, 0.50, d.Aggression)
	for _, squadName := range []string{"ground-attack", "naval-attack"} {
		rules = append(rules, &Rule{
			Name:         fmt.Sprintf("recall-overextended-%s", squadName),
			Priority:     retreatPriority - 10,
			Category:     "micro",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("%s") && len(OverextendedSquadMembers("%s", %.2f)) > 0`, squadName, squadName, leashPct),
			Action:       RecallOverextended(squadName, leashPct),
		})
	}

	// Engagement quality — disengage when outnumbered locally.
	// Only active when aggression < 1.0 (pure aggression doctrines never retreat).
	if d.Aggression < 1.0 {
		// threatThreshold: 1.5 at aggression=0 (cautious), 3.0 at aggression=0.9 (aggressive)
		threatThreshold := lerpf(1.5, 3.0, d.Aggression)
		checkRadius := 0.10 // 10% of map diagonal
		for _, squadName := range []string{"ground-attack", "naval-attack"} {
			rules = append(rules, &Rule{
				Name:         fmt.Sprintf("squad-disengage-%s", squadName),
				Priority:     retreatPriority - 5,
				Category:     "micro",
				Exclusive:    false,
				ConditionSrc: fmt.Sprintf(`SquadExists("%s") && SquadThreatRatio("%s", %.2f) > %.2f`, squadName, squadName, checkRadius, threatThreshold),
				Action:       SquadDisengage(squadName),
			})
		}
	}

	// Focus fire on weakest visible enemy — aggressive doctrines only.
	if d.Aggression > DoctrineModerate {
		rules = append(rules, &Rule{
			Name:         "squad-focus-fire",
			Priority:     attackPriority + 1,
			Category:     "micro",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`SquadExists("ground-attack") && SquadReadyRatio("ground-attack") >= %.2f && BestGroundTarget() != nil`, activationThreshold),
			Action:       SquadFocusFire("ground-attack"),
		})
	}

	// Flee harvesters from danger — economy-focused doctrines.
	if d.EconomyPriority > DoctrineEnabled {
		dangerPct := lerpf(0.05, 0.15, d.EconomyPriority)
		fleePriority := lerp(150, 300, d.EconomyPriority)
		rules = append(rules, &Rule{
			Name:         "flee-harvesters",
			Priority:     fleePriority,
			Category:     "micro",
			Exclusive:    false,
			ConditionSrc: fmt.Sprintf(`EnemiesVisible() && len(HarvestersInDanger(%.2f)) > 0`, dangerPct),
			Action:       FleeHarvesters(dangerPct),
		})
	}

	// --- Recon ---
	// Rangers are dedicated scouts — sent immediately when idle. They use Move
	// (fast, no stopping to fight) and fan out to different waypoints.
	// Always patrol: maintains map vision and refreshes enemy intel. Without
	// this, rangers go permanently idle once intel is gathered and enemies
	// are visible — and they're excluded from combat rules.

	scoutPriority := lerp(250, 400, d.ScoutPriority)
	rules = append(rules, &Rule{
		Name:         "scout-with-rangers",
		Priority:     scoutPriority + SquadFormBonus,
		Category:     "recon",
		Exclusive:    false,
		ConditionSrc: `len(IdleRangers()) > 0`,
		Action:       ActionScoutWithRangers,
	})

	// Generic fallback: scout with any idle ground units once army is large enough.
	// Gates on visibility, not intel — having historical intel doesn't mean we
	// know where enemies are now. Scouts resume patrolling whenever enemies
	// leave vision, preventing idle stalls at reached waypoints.
	rules = append(rules, &Rule{
		Name:         "scout-with-idle-units",
		Priority:     scoutPriority,
		Category:     "recon",
		Exclusive:    false,
		ConditionSrc: fmt.Sprintf(`!EnemiesVisible() && len(UnassignedIdleGround()) >= %d`, d.GroundAttackGroupSize),
		Action:       ActionScoutWithIdleUnits,
	})

	return rules
}
