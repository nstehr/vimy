package agent

import (
	"fmt"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
)

// EventKind identifies the category of a game event that should trigger
// doctrine re-evaluation by the strategist.
type EventKind string

const (
	EventCriticalBuildingLost EventKind = "critical_building_lost"
	EventArmyDevastated       EventKind = "army_devastated"
	EventEnemyBaseDiscovered  EventKind = "enemy_base_discovered"
	EventPhaseTransition      EventKind = "phase_transition"
	EventEconomyCrisis        EventKind = "economy_crisis"
	EventSuperweaponReady     EventKind = "superweapon_ready"
	EventFirstContact         EventKind = "first_contact"
	EventStrategyCountered    EventKind = "strategy_countered"
)

// Event represents a significant game event detected by diffing consecutive
// game states. Events are accumulated on the strategist and included in the
// LLM situation summary so it knows *why* it's being asked to re-evaluate.
type Event struct {
	Kind   EventKind
	Tick   int
	Detail string // human-readable description for the LLM
}

// stateSnapshot captures the diffable fields from a game state tick.
// The strategist stores one and compares against the next tick to detect events.
type stateSnapshot struct {
	buildingIDs  map[int]string // id → type for owned buildings
	combatCount  int            // number of non-economic, non-harvester units
	harvesterCnt int
	cash         int
	phase        string // "Early Game", "Mid Game", "Late Game"
	hasEnemyBase bool   // building-based enemy intel exists
	superReady   map[string]bool
	enemiesSeen  bool // any enemies visible

	// Per-domain unit tracking for strategy_countered detection
	infantryIDs map[int]bool
	vehicleIDs  map[int]bool
	aircraftIDs map[int]bool

	// Cooldown: tick of last strategy_countered event (carried forward by strategist)
	lastCounterTick int

	// lossBaselineTick is when the domain ID accumulation window started.
	// Within the window, domain ID sets grow (new units added) but dead units
	// stay in the set so countMissing reflects accumulated losses.
	lossBaselineTick int
}

// criticalBuildingTypes are buildings whose loss fundamentally changes
// what the AI can do and should trigger immediate re-evaluation.
var criticalBuildingTypes = map[string]bool{
	"fact": true, // Construction Yard
	"weap": true, // War Factory
	"atek": true, // Allied Tech Center
	"stek": true, // Soviet Tech Center
	"proc": true, // Refinery
	"mslo": true, // Missile Silo
	"iron": true, // Iron Curtain
}

// baseType strips faction variants (e.g. "fact.england" → "fact") and lowercases.
func baseType(t string) string {
	base := strings.ToLower(t)
	if idx := strings.IndexByte(base, '.'); idx >= 0 {
		base = base[:idx]
	}
	return base
}

// isCriticalBuilding handles faction variants (e.g. "fact.england" → "fact").
func isCriticalBuilding(t string) bool {
	return criticalBuildingTypes[baseType(t)]
}

// Unit domain classification — which of our units belong to each combat domain.
var infantryTypes = map[string]bool{
	"e1": true, "e3": true, "e4": true, "e6": true, "e7": true,
	"shok": true, "medi": true,
}

var vehicleTypes = map[string]bool{
	"1tnk": true, "2tnk": true, "3tnk": true, "4tnk": true,
	"v2rl": true, "apc": true, "ftrk": true, "dtrk": true,
	"jeep": true, "arty": true,
}

var aircraftTypes = map[string]bool{
	"heli": true, "mh60": true, "mig": true, "yak": true,
}

// Enemy threat classification — what counters each domain.
var antiInfantryThreats = map[string]bool{
	"ftur": true, // Flame Tower
	"tsla": true, // Tesla Coil
	"pbox": true, // Pillbox
	"hbox": true, // Heavy Pillbox
	"e4":   true, // Flamethrower infantry
	"shok": true, // Shock Trooper
}

var antiVehicleThreats = map[string]bool{
	"tsla": true, // Tesla Coil
	"gun":  true, // Gun Turret
}

var antiAirThreats = map[string]bool{
	"sam":  true, // SAM Site
	"agun": true, // AA Gun
	"ftrk": true, // Flak Truck
}

// threatDisplayName maps internal type codes to human-readable names for the LLM.
var threatDisplayName = map[string]string{
	"ftur": "Flame Tower", "tsla": "Tesla Coil",
	"pbox": "Pillbox", "hbox": "Heavy Pillbox", "gun": "Turret",
	"sam": "SAM Site", "agun": "AA Gun",
	"e4": "Flamethrower", "shok": "Shock Trooper", "ftrk": "Flak Truck",
}

// counterCooldownTicks is the minimum gap between strategy_countered events.
const counterCooldownTicks = 200

// counterLossThresholds is the minimum units lost per domain to trigger the event.
// Infantry dies much faster than vehicles/aircraft, so a lower threshold
// ensures the event fires sooner for infantry swarms getting shredded.
var counterLossThresholds = map[string]int{
	"infantry": 2,
	"vehicle":  3,
	"aircraft": 2,
}

// Mid-game building types: real military production capability.
var midGameBuildings = map[string]bool{
	"weap": true, // War Factory
	"afld": true, // Airfield
	"syrd": true, // Naval Yard
}

// barracksTypes identifies barracks buildings (Allied tent / Soviet barr).
var barracksTypes = map[string]bool{
	"tent": true, // Allied Barracks
	"barr": true, // Soviet Barracks
}

// Late-game building types: advanced tech or superweapons.
var lateGameBuildings = map[string]bool{
	"atek": true, // Allied Tech Center
	"stek": true, // Soviet Tech Center
	"mslo": true, // Missile Silo
	"iron": true, // Iron Curtain
}

// gamePhase determines the game phase from building milestones, with tick
// as a generous fallback for stalled games. Tick-only thresholds caused the
// LLM to think it was mid-game when only barracks and a few troops existed.
func gamePhase(gs model.GameState) string {
	hasLate := false
	hasMid := false
	hasBarracks := false
	for _, b := range gs.Buildings {
		bt := baseType(b.Type)
		if lateGameBuildings[bt] {
			hasLate = true
			break
		}
		if midGameBuildings[bt] {
			hasMid = true
		}
		if barracksTypes[bt] {
			hasBarracks = true
		}
	}

	if hasLate || gs.Tick > 5000 {
		return "Late Game"
	}
	if hasMid || gs.Tick > 2000 {
		return "Mid Game"
	}

	// Barracks + 5 combat units = valid mid-game (infantry rush).
	// Just barracks + 2 troops stays "Early Game" to avoid premature transitions.
	if hasBarracks {
		combatCount := 0
		for _, u := range gs.Units {
			if isCombatUnit(u) {
				combatCount++
			}
		}
		if combatCount >= 5 {
			return "Mid Game"
		}
	}

	return "Early Game"
}

// isCombatUnit returns true if the unit is a combat unit (not harvester or MCV).
func isCombatUnit(u model.Unit) bool {
	t := baseType(u.Type)
	return t != "harv" && t != "mcv"
}

// unitDomain returns "infantry", "vehicle", "aircraft", or "" for the unit.
func unitDomain(u model.Unit) string {
	t := baseType(u.Type)
	if infantryTypes[t] {
		return "infantry"
	}
	if vehicleTypes[t] {
		return "vehicle"
	}
	if aircraftTypes[t] {
		return "aircraft"
	}
	return ""
}

// takeSnapshot captures the current diffable state for next tick's comparison.
func takeSnapshot(gs model.GameState, memory map[string]any) stateSnapshot {
	snap := stateSnapshot{
		buildingIDs:  make(map[int]string, len(gs.Buildings)),
		cash:         gs.Player.Cash + gs.Player.Resources,
		phase:        gamePhase(gs),
		enemiesSeen:  len(gs.Enemies) > 0,
		superReady:   make(map[string]bool),
		harvesterCnt: 0,
		infantryIDs:  make(map[int]bool),
		vehicleIDs:   make(map[int]bool),
		aircraftIDs:  make(map[int]bool),
	}

	for _, b := range gs.Buildings {
		snap.buildingIDs[b.ID] = b.Type
	}

	for _, u := range gs.Units {
		if isCombatUnit(u) {
			snap.combatCount++
		}
		t := baseType(u.Type)
		if t == "harv" {
			snap.harvesterCnt++
		}
		switch unitDomain(u) {
		case "infantry":
			snap.infantryIDs[u.ID] = true
		case "vehicle":
			snap.vehicleIDs[u.ID] = true
		case "aircraft":
			snap.aircraftIDs[u.ID] = true
		}
	}

	for _, sp := range gs.SupportPowers {
		if sp.Ready {
			snap.superReady[sp.Key] = true
		}
	}

	// Check for building-based enemy intel
	if bases, ok := memory["enemyBases"].(map[string]rules.EnemyBaseIntel); ok {
		for _, base := range bases {
			if base.FromBuildings {
				snap.hasEnemyBase = true
				break
			}
		}
	}

	return snap
}

// detectEvents compares the current game state against the previous snapshot
// and returns any triggered events. Returns nil if prev is nil (first tick).
func detectEvents(gs model.GameState, memory map[string]any, prev *stateSnapshot) []Event {
	if prev == nil {
		return nil
	}

	var events []Event
	cur := takeSnapshot(gs, memory)

	// 1. critical_building_lost: a critical building present last tick is now gone
	for id, typ := range prev.buildingIDs {
		if isCriticalBuilding(typ) {
			if _, exists := cur.buildingIDs[id]; !exists {
				events = append(events, Event{
					Kind:   EventCriticalBuildingLost,
					Tick:   gs.Tick,
					Detail: fmt.Sprintf("Lost critical building: %s (id %d)", typ, id),
				})
				break // one event per tick is enough
			}
		}
	}

	// 2. army_devastated: >50% combat units lost (floor of 6 to avoid early noise)
	if prev.combatCount >= 6 && cur.combatCount > 0 {
		lost := prev.combatCount - cur.combatCount
		if lost > 0 && float64(lost)/float64(prev.combatCount) > 0.5 {
			events = append(events, Event{
				Kind:   EventArmyDevastated,
				Tick:   gs.Tick,
				Detail: fmt.Sprintf("Army devastated: %d→%d combat units (lost %d%%)", prev.combatCount, cur.combatCount, 100*lost/prev.combatCount),
			})
		}
	}

	// 3. enemy_base_discovered: first building-based enemy intel
	if !prev.hasEnemyBase && cur.hasEnemyBase {
		events = append(events, Event{
			Kind:   EventEnemyBaseDiscovered,
			Tick:   gs.Tick,
			Detail: "Enemy base discovered via building sighting",
		})
	}

	// 4. phase_transition: Early→Mid (tick 1000) or Mid→Late (tick 3000)
	if prev.phase != cur.phase {
		events = append(events, Event{
			Kind:   EventPhaseTransition,
			Tick:   gs.Tick,
			Detail: fmt.Sprintf("Phase transition: %s → %s", prev.phase, cur.phase),
		})
	}

	// 5. economy_crisis: all harvesters lost, or cash collapses from >1000 to <200
	if prev.harvesterCnt > 0 && cur.harvesterCnt == 0 {
		events = append(events, Event{
			Kind:   EventEconomyCrisis,
			Tick:   gs.Tick,
			Detail: "Economy crisis: all harvesters lost",
		})
	} else if prev.cash > 1000 && cur.cash < 200 {
		events = append(events, Event{
			Kind:   EventEconomyCrisis,
			Tick:   gs.Tick,
			Detail: fmt.Sprintf("Economy crisis: cash collapsed %d → %d", prev.cash, cur.cash),
		})
	}

	// 6. superweapon_ready: our nuke or iron curtain became ready
	for key, ready := range cur.superReady {
		if ready && !prev.superReady[key] {
			events = append(events, Event{
				Kind:   EventSuperweaponReady,
				Tick:   gs.Tick,
				Detail: fmt.Sprintf("Superweapon ready: %s", key),
			})
		}
	}

	// 7. first_contact: enemies visible for the first time
	if !prev.enemiesSeen && cur.enemiesSeen {
		events = append(events, Event{
			Kind:   EventFirstContact,
			Tick:   gs.Tick,
			Detail: fmt.Sprintf("First contact: %d enemies now visible", len(gs.Enemies)),
		})
	}

	// 8. strategy_countered: forces being hard-countered by enemy composition.
	// Fires when we lose 3+ units in a domain AND relevant enemy counter-threats
	// are visible. 200-tick cooldown prevents spam during prolonged battles.
	if prev.lastCounterTick == 0 || gs.Tick-prev.lastCounterTick >= counterCooldownTicks {
		type domainCheck struct {
			name    string
			prevIDs map[int]bool
			curIDs  map[int]bool
			threats map[string]bool
		}
		checks := []domainCheck{
			{"infantry", prev.infantryIDs, cur.infantryIDs, antiInfantryThreats},
			{"vehicle", prev.vehicleIDs, cur.vehicleIDs, antiVehicleThreats},
			{"aircraft", prev.aircraftIDs, cur.aircraftIDs, antiAirThreats},
		}

		var countered []string
		for _, c := range checks {
			lost := countMissing(c.prevIDs, c.curIDs)
			threshold := counterLossThresholds[c.name]
			if threshold == 0 {
				threshold = 3
			}
			if lost < threshold {
				continue
			}
			threats := visibleThreats(gs.Enemies, c.threats)
			if len(threats) == 0 {
				continue
			}
			countered = append(countered, fmt.Sprintf("%s taking heavy losses (%d killed); enemy has %s",
				c.name, lost, formatThreats(threats)))
		}

		if len(countered) > 0 {
			events = append(events, Event{
				Kind:   EventStrategyCountered,
				Tick:   gs.Tick,
				Detail: strings.Join(countered, "; "),
			})
		}
	}

	return events
}

// countMissing returns how many IDs in prev are absent from cur.
func countMissing(prev, cur map[int]bool) int {
	n := 0
	for id := range prev {
		if !cur[id] {
			n++
		}
	}
	return n
}

// visibleThreats counts enemy units/buildings matching the threat set.
func visibleThreats(enemies []model.Enemy, threatSet map[string]bool) map[string]int {
	counts := make(map[string]int)
	for _, e := range enemies {
		t := baseType(e.Type)
		if threatSet[t] {
			counts[t]++
		}
	}
	return counts
}

// formatThreats renders a threat count map as "2x Flame Tower, 1x Tesla Coil".
func formatThreats(threats map[string]int) string {
	parts := make([]string, 0, len(threats))
	for t, count := range threats {
		name := threatDisplayName[t]
		if name == "" {
			name = t
		}
		parts = append(parts, fmt.Sprintf("%dx %s", count, name))
	}
	return strings.Join(parts, ", ")
}

// mergeIDSets returns the union of two ID sets. Used to accumulate domain
// unit IDs across ticks so that losses add up over the observation window
// instead of resetting each tick.
func mergeIDSets(a, b map[int]bool) map[int]bool {
	merged := make(map[int]bool, len(a)+len(b))
	for id := range a {
		merged[id] = true
	}
	for id := range b {
		merged[id] = true
	}
	return merged
}

// formatEvents renders accumulated events as a "Recent Events" section
// for the LLM situation summary.
func formatEvents(events []Event) string {
	if len(events) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nRecent Events:\n")
	for _, e := range events {
		fmt.Fprintf(&b, "- [tick %d] %s: %s\n", e.Tick, e.Kind, e.Detail)
	}
	return b.String()
}
