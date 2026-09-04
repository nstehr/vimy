package rules

import "fmt"

// addBuildingRules emits rules for prerequisite buildings (radar, barracks,
// war factory), military production buildings (airfield, naval yard, service
// depot), defenses (ground, AA, gap generator), tech progression, superweapon
// buildings, and extra production buildings.
func (c *doctrineCompiler) addBuildingRules() {
	// --- Prerequisite buildings ---

	// Radar is the tech-tree gate for vehicles, aircraft, and naval —
	// include it whenever any of those paths are desired. Priority is set
	// below all military-building priorities (war factory / airfield /
	// naval yard lerp 580–680, barracks 600–700, barracks-prereq 600) so
	// the doctrine's actual production building always wins the exclusive
	// "economy" queue first. Radar slots in after — it's a prerequisite
	// for spy / siege / tech-center only, none of which matter before a
	// production building exists. This keeps rush doctrines from burning
	// 1000 cash on radar before the war factory is up.
	if c.d.VehicleWeight > DoctrineEnabled || c.d.AirWeight > DoctrineEnabled || c.d.NavalWeight > DoctrineEnabled || c.d.TechPriority > DoctrineSignificant {
		// Default priority 570 keeps military buildings ahead of radar —
		// APC/engineer rush doctrines depend on this so the war factory is
		// built before the 1000-cash radar. But when the doctrine's PRIMARY
		// preferred vehicle requires radar (V2, artillery, heavy/medium/
		// tesla tank, mammoth), radar IS the enablement step for combat
		// production. Without this bump V2 doctrines wait ~6000 ticks for
		// radar and the entire combat pipeline is dead until then. Floor
		// 710 puts radar just below the siege-preferred war-factory floor
		// (720), giving the human order: power → refinery → war factory →
		// radar → second refinery → …
		//
		// Narrowly gated on PreferredVehicle[0]: APC-rush doctrines list
		// 'apc' first (not radar-gated) even when they name medium_tank /
		// heavy_tank as secondary prefs, so this bump doesn't regress them.
		radarPriority := 570
		if prefersRadarGatedPrimary(c.d.PreferredVehicle) {
			radarPriority = 710
		}
		c.rules = append(c.rules, &Rule{
			Name:         "build-radar",
			Priority:     radarPriority,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!IsRushed() && !QueueBusy("Building") && CanBuildRole("radar") && !HasRole("radar") && !QueueProducingRole("radar") && (HasRole("barracks") || HasRole("war_factory")) && PowerExcess() >= 0 && Cash() >= 1000`,
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
		// Barracks priority floor. Barracks is $300 and unblocks the entire
		// infantry queue — it should always come before radar (710 max) and
		// normal-cost war factory (680-730), but after power (800) and first
		// refinery (750). Game 63 observed radar+advanced-power+WF beating
		// barracks to the queue when medium_tank was preferred, leaving
		// hasBarracks=false for 5000 ticks and blocking all rifle production.
		// TA-boosted war factory (770+) can still legitimately outrank when
		// the doctrine is truly APC-first, so the floor doesn't overreach.
		if barracksPriority < 745 {
			barracksPriority = 745
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
		// Lerp ceiling 730 (was 680) so meaningfully vehicle-focused doctrines
		// can actually beat build-second-refinery (ceiling 700 at max
		// EconomyPriority). Gradient preserved: a VehicleWeight=0.6 doctrine
		// gets ~670 (beats second-refinery at EconomyPriority<=0.9), a
		// VehicleWeight=1.0 doctrine gets 730 (always beats second-refinery).
		// Moderate doctrines (VehicleWeight 0.2-0.4) still land around
		// 610-640 — economy wins, which is correct for a secondary-vehicle
		// focus. Ceiling stays below build-refinery (750) / build-power (800)
		// so first refinery and power still come first.
		warFactoryPriority := lerp(580, 730, c.d.VehicleWeight)
		// Transport assault doctrines need a war factory ASAP for APCs.
		// Boost priority so the war factory doesn't lose to barracks in the
		// exclusive "economy" category.
		if c.d.TransportAssault > DoctrineModerate {
			taBoost := lerp(0, 40, c.d.TransportAssault)
			warFactoryPriority = max(warFactoryPriority, lerp(600, 700, c.d.TransportAssault))
			warFactoryPriority += taBoost
		}
		// Standoff-bombardment doctrines (prefer v2_launcher / artillery) have
		// the narrowest critical path — combat depends on WF + radar only.
		// Floor 720 so they reliably win regardless of what EconomyPriority
		// the strategist paired with the doctrine.
		if c.prefersVehicle("v2_launcher") || c.prefersVehicle("artillery") {
			warFactoryPriority = max(warFactoryPriority, 720)
		}
		// War factory must come before specialized production yards (naval
		// yard, airfield) regardless of doctrine. Vehicles are universally
		// useful — they produce harvesters, defenders, MCV recovery, V2s,
		// and APCs for engineer rushes — while naval and air are
		// map-conditional. Game 13 (russia vs germany, naval directive,
		// lost 19m) showed the failure mode: nav=0.70 in the opener gave
		// build-naval-yard ~650 priority, beating build-war-factory at
		// ~633. The naval yard built first, the war factory never did,
		// the rush hit, and zero tanks were ever produced. Floor war
		// factory at 685 (above the 680 ceiling on both build-naval-yard
		// and build-airfield) whenever those alternatives compile, so
		// the universal building always wins the build slot first.
		if c.d.NavalWeight > DoctrineEnabled || c.d.AirWeight > DoctrineEnabled {
			warFactoryPriority = max(warFactoryPriority, 685)
		}
		// Scale cash threshold inversely with vehicle weight: low-vehicle
		// doctrines need a bigger buffer so the 2000-credit building doesn't
		// starve air/naval production during construction. Ceiling capped at
		// 2500 (was 3500) — the old value made it nearly impossible for rush
		// doctrines to accumulate enough cash since infantry production drained
		// funds below the threshold.
		wfCashThreshold := lerp(2500, 2000, c.d.VehicleWeight)
		// Transport assault needs a cheaper threshold since the whole strategy
		// depends on getting APCs out quickly.
		if c.d.TransportAssault > DoctrineModerate {
			wfCashThreshold = min(wfCashThreshold, lerp(2200, 2000, c.d.TransportAssault))
		}
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
			ConditionSrc: `!IsRushed() && !QueueBusy("Building") && CanBuildRole("airfield") && !HasRole("airfield") && PowerExcess() >= 0 && Cash() >= 500`,
			Action:       ActionProduceAirfield,
		})
	}

	if c.d.VehicleWeight > DoctrineSignificant {
		c.rules = append(c.rules, &Rule{
			Name:         "build-service-depot",
			Priority:     570,
			Category:     "economy",
			Exclusive:    true,
			ConditionSrc: `!IsRushed() && !QueueBusy("Building") && CanBuildRole("service_depot") && !HasRole("service_depot") && HasRole("war_factory") && PowerExcess() >= 0 && Cash() >= 1200`,
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
			ConditionSrc: `!IsRushed() && MapHasWater() && !QueueBusy("Building") && CanBuildRole("naval_yard") && !HasRole("naval_yard") && PowerExcess() >= 0 && Cash() >= 500`,
			Action:       ActionProduceNavalYard,
		})
	}

	// --- Ground defenses ---

	if c.d.GroundDefensePriority > DoctrineModerate {
		// vimy-90o: cap was 1..5 (gd=1.0 → only 5 total ground defenses), too
		// few to cover multiple entry points when raids destroy them as fast
		// as we rebuild. Bumped to 2..10 so a well-defended doctrine can hold
		// a real perimeter line. Cash gate still throttles spend.
		defenseCap := lerp(2, 10, c.d.GroundDefensePriority)
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

		// Rush-mode variant (vimy-ovm follow-up): when being_rushed, lower the
		// cash floor to 200 (a pillbox costs 400 — the floor is hold-back, not
		// the build cost itself) and prioritize above other economy builds so
		// defenses go up before the next refinery/power plant. Without this,
		// build-base-defense competes with refineries on cost and loses; in
		// game 33/34 only 9 defenses got built across 22-30k ticks of raids.
		//
		// No production-building reserves during rush (was in vimy-qp1,
		// removed after game 47). Pillboxes matter for surviving the rush
		// window; WF/airfield build after the rush clears via their own
		// build rules.
		c.rules = append(c.rules, &Rule{
			Name:         "build-base-defense-rush",
			Priority:     defensePriority + 100,
			Category:     "defense",
			Exclusive:    true,
			ConditionSrc: fmt.Sprintf(`IsRushed() && !QueueBusy("Defense") && PowerExcess() >= 0 && (CanBuildRole("pillbox") || CanBuildRole("camo_pillbox") || CanBuildRole("turret") || CanBuildRole("flame_tower") || CanBuildRole("tesla_coil")) && (RoleCount("pillbox") + RoleCount("camo_pillbox") + RoleCount("turret") + RoleCount("flame_tower") + RoleCount("tesla_coil")) < %d && Cash() >= 200`, defenseCap),
			Action:       ActionProduceDefense,
		})
	}

	// --- AA defenses ---

	if c.d.AirDefensePriority > DoctrineSignificant {
		// vimy-90o: 1..3 was too few for a base with multiple aircraft
		// approach vectors. 2..5 lets ad=1.0 cover both flanks plus the rear.
		aaCap := lerp(2, 5, c.d.AirDefensePriority)
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
			ConditionSrc: fmt.Sprintf(`!IsRushed() && !QueueBusy("Defense") && PowerExcess() >= 0 && CanBuildRole("gap_generator") && HasRole("tech_center") && RoleCount("gap_generator") < %d && Cash() >= 800`, gapCap),
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
			ConditionSrc: `!IsRushed() && !QueueBusy("Building") && CanBuildRole("tech_center") && !HasRole("tech_center") && HasRole("radar") && PowerExcess() >= 0 && Cash() >= 1500`,
			Action:       ActionProduceTechCenter,
		})
	}

	// --- Superweapon buildings ---

	if c.d.SuperweaponPriority > DoctrineSignificant {
		c.rules = append(c.rules, &Rule{
			Name:         "build-missile-silo",
			Priority:     650,
			Category:     "superweapon-build",
			Exclusive:    true,
			ConditionSrc: `!IsRushed() && !QueueBusy("Defense") && CanBuildRole("missile_silo") && !HasRole("missile_silo") && HasRole("tech_center") && PowerExcess() >= 0 && Cash() >= 2500`,
			Action:       ActionProduceMissileSilo,
		})

		c.rules = append(c.rules, &Rule{
			Name:         "build-iron-curtain",
			Priority:     640,
			Category:     "superweapon-build",
			Exclusive:    true,
			ConditionSrc: `!IsRushed() && !QueueBusy("Defense") && CanBuildRole("iron_curtain") && !HasRole("iron_curtain") && HasRole("tech_center") && PowerExcess() >= 0 && Cash() >= 2500`,
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
			ConditionSrc: fmt.Sprintf(`!IsRushed() && !QueueBusy("Building") && CanBuildRole("barracks") && RoleCount("barracks") < %d && PowerExcess() >= 0 && Cash() >= 300`, extraBarracksCap),
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
			ConditionSrc: fmt.Sprintf(`!IsRushed() && !QueueBusy("Building") && CanBuildRole("war_factory") && RoleCount("war_factory") < %d && PowerExcess() >= 0 && Cash() >= 2000`, extraWFCap),
			Action:       ActionProduceWarFactory,
		})
	}

	// Extra airfield/helipad building. Compiled whenever the doctrine cares
	// about air at all (was DoctrineExtreme=0.4, which was too high — a
	// doctrine at air_weight 0.35 wanted 3 aircraft but only had 1 pad).
	// Gated on physical AircraftCapacity vs the doctrinal aircraft cap: if
	// physical pads can't hold the doctrine's aircraft count, keep building
	// pads until they can. Fires only when existing pads are near-full so
	// we don't overbuild speculatively.
	if c.d.AirWeight > DoctrineEnabled {
		airCap := lerp(2, 8, c.d.AirWeight)
		c.rules = append(c.rules, &Rule{
			Name:      "build-extra-airfield",
			Priority:  480,
			Category:  "economy",
			Exclusive: true,
			// AircraftCapacity < doctrinal cap: still need more pads.
			// CombatAircraftCount >= AircraftCapacity - 1: existing pads are
			// actually filling up (don't build speculatively). Together this
			// grows pads to match the doctrine's aircraft ambition without
			// leaving Allied doctrines (1 pad per helipad) permanently short.
			ConditionSrc: fmt.Sprintf(`!IsRushed() && !QueueBusy("Building") && CanBuildRole("airfield") && AircraftCapacity() < %d && CombatAircraftCount() >= AircraftCapacity() - 1 && PowerExcess() >= 0 && Cash() >= 500`, airCap),
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
			ConditionSrc: fmt.Sprintf(`!IsRushed() && MapHasWater() && !QueueBusy("Building") && CanBuildRole("naval_yard") && RoleCount("naval_yard") < %d && (RoleCount("submarine") + RoleCount("destroyer")) >= %d && PowerExcess() >= 0 && Cash() >= 500`, extraNavalCap, navalCapForGate-1),
			Action:       ActionProduceNavalYard,
		})
	}
}
