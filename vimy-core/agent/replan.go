package agent

import (
	"math"
	"strings"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
)

// stressEventTTL keeps a high-impact event in the feed for roughly three
// evaluation intervals — the pivot itself plus a follow-up, before the model is
// allowed to consider reverting.
const stressEventTTL = 1500

// stressKinds are the events signalling pressure a single pivot rarely
// resolves. Lower-impact kinds stay out so the persisted feed stays tight.
var stressKinds = map[EventKind]bool{
	EventCriticalBuildingLost: true,
	EventArmyDevastated:       true,
	EventEconomyCrisis:        true,
	EventStrategyCountered:    true,
	EventHarvesterLost:        true,
	EventHarvesterUnderAttack: true,
}

// pruneStressEvents drops persisted events older than stressEventTTL.
func pruneStressEvents(buf []Event, currentTick int) []Event {
	if len(buf) == 0 {
		return buf
	}
	out := buf[:0]
	for _, e := range buf {
		if currentTick-e.Tick <= stressEventTTL {
			out = append(out, e)
		}
	}
	return out
}

// appendStressEvents copies stress-kind entries from fresh events into the
// persisted buffer, deduping by (Kind, Tick).
func appendStressEvents(buf []Event, fresh []Event) []Event {
	for _, e := range fresh {
		if !stressKinds[e.Kind] {
			continue
		}
		dup := false
		for _, b := range buf {
			if b.Kind == e.Kind && b.Tick == e.Tick {
				dup = true
				break
			}
		}
		if !dup {
			buf = append(buf, e)
		}
	}
	return buf
}

// mergeStressEvents dedupes fresh and persisted events by (Kind, Tick), so the
// LLM sees this tick's events alongside recent unresolved stress.
func mergeStressEvents(fresh, persisted []Event) []Event {
	if len(persisted) == 0 {
		return fresh
	}
	out := append([]Event(nil), fresh...)
	for _, p := range persisted {
		dup := false
		for _, f := range fresh {
			if f.Kind == p.Kind && f.Tick == p.Tick {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	return out
}

// axisDominantThreshold is where a doctrine counts as pivoting to an axis.
// Mirrors the convergence-break wording in the prompt.
const axisDominantThreshold = 0.55

// burnedAxisMinPivots is how many countered pivots burn an axis. Air needs only
// one: SAM and flak clusters are effectively permanent. The others need two, so
// a single bad engagement — rifles into one tesla — doesn't abandon the axis.
var burnedAxisMinPivots = map[string]int{
	"air":      1,
	"infantry": 2,
	"vehicle":  2,
	"naval":    2,
}

// burnedAxisLookbackTicks is how long after a pivot a counter still counts
// against it — the same window stress events persist for.
const burnedAxisLookbackTicks = 1500

// burnedAxisRecoveryTicks un-burns an axis after a counter-free period; 0 means
// the burn holds for the match. Only air recovers: AA can be suppressed, and
// without recovery one early SAM kill neuters an air-superiority directive for
// the remaining 100k ticks. Ground counter-tech only stacks up over a match.
var burnedAxisRecoveryTicks = map[string]int{
	"air": 5000,
}

// computeBurnedAxes returns axes pivoted to at least burnedAxisMinPivots times,
// each pivot followed by a domain-specific counter event. Downstream this is a
// hard constraint, not a suggestion.
//
// Over the full history rather than a recent window: repeat air pivots are
// routinely 10-20 doctrines apart and a short window catches none of them.
func computeBurnedAxes(history []DoctrineRecord, stress []Event, currentTick int) []string {
	if len(history) == 0 {
		return nil
	}
	axisCounts := map[string]int{}
	axisLatestCounter := map[string]int{}
	for _, rec := range history {
		d := rec.Doctrine
		var axis string
		switch {
		case d.AirWeight >= axisDominantThreshold:
			axis = "air"
		case d.InfantryWeight >= axisDominantThreshold:
			axis = "infantry"
		case d.NavalWeight >= axisDominantThreshold:
			axis = "naval"
		case d.VehicleWeight >= axisDominantThreshold:
			axis = "vehicle"
		default:
			continue
		}
		if t, ok := latestAxisCounter(rec.Tick, axis, stress); ok {
			axisCounts[axis]++
			if t > axisLatestCounter[axis] {
				axisLatestCounter[axis] = t
			}
		}
	}
	var out []string
	for axis, n := range axisCounts {
		threshold, ok := burnedAxisMinPivots[axis]
		if !ok {
			threshold = 2
		}
		if n < threshold {
			continue
		}
		// An axis whose latest counter has aged out of its recovery window
		// un-burns.
		if rt, has := burnedAxisRecoveryTicks[axis]; has && rt > 0 {
			if currentTick-axisLatestCounter[axis] > rt {
				continue
			}
		}
		out = append(out, axis)
	}
	return out
}

// latestAxisCounter returns the tick of the most recent matching stress event
// within the lookback window after pivotTick.
func latestAxisCounter(pivotTick int, axis string, stress []Event) (int, bool) {
	latest := 0
	found := false
	for _, e := range stress {
		if e.Tick < pivotTick || e.Tick-pivotTick > burnedAxisLookbackTicks {
			continue
		}
		if !eventMatchesAxis(e, axis) {
			continue
		}
		if e.Tick > latest {
			latest = e.Tick
		}
		found = true
	}
	return latest, found
}

// eventMatchesAxis is shared by latestAxisCounter and axisCounteredAfter.
func eventMatchesAxis(e Event, axis string) bool {
	detail := strings.ToLower(e.Detail)
	switch axis {
	case "air":
		return strings.Contains(detail, "aircraft") || strings.Contains(detail, "sam") || strings.Contains(detail, "flak")
	case "infantry":
		return strings.Contains(detail, "infantry") || strings.Contains(detail, "flame") || strings.Contains(detail, "tesla")
	case "vehicle":
		return e.Kind == EventArmyDevastated
	case "naval":
		return strings.Contains(detail, "naval") || strings.Contains(detail, "submarine") || strings.Contains(detail, "destroyer")
	}
	return false
}

// axisCounteredAfter reports whether a matching stress event fired inside the
// lookback window after a pivot. Matching is on the detail string, which is
// where strategy_countered events carry their domain.
func axisCounteredAfter(pivotTick int, axis string, stress []Event) bool {
	for _, e := range stress {
		if e.Tick < pivotTick || e.Tick-pivotTick > burnedAxisLookbackTicks {
			continue
		}
		detail := strings.ToLower(e.Detail)
		switch axis {
		case "air":
			if strings.Contains(detail, "aircraft") || strings.Contains(detail, "sam") || strings.Contains(detail, "flak") {
				return true
			}
		case "infantry":
			if strings.Contains(detail, "infantry") || strings.Contains(detail, "flame") || strings.Contains(detail, "tesla") {
				return true
			}
		case "vehicle":
			if e.Kind == EventArmyDevastated {
				return true
			}
		case "naval":
			if strings.Contains(detail, "naval") || strings.Contains(detail, "submarine") || strings.Contains(detail, "destroyer") {
				return true
			}
		}
	}
	return false
}

// Pressure-flag thresholds. Harvesters under attack is not one signal: in a
// rush they always are, because there is no perimeter yet. The two cases want
// opposite responses, so they are separate flags:
//
//   - being_rushed: early game. Counter-force — killing the rushers is what
//     unblocks the economy, not bunkering.
//   - harvester_harassed: sustained losses against an established base.
//     Dispersal, escorts and static defense, at some cost to the main push.
const (
	// Wide enough for slow-paced rushes: a first harvester attack near tick
	// 10000 against a still-small base is a rush, and a tighter window sends it
	// to the softer harassment response instead.
	rushTickCutoff = 10500
	// No building-count gate: vimy routinely clears any plausible threshold
	// before raids arrive, so the condition only made the flag silently false.
	// The tick window already means "early game", and the rush rules cap their
	// own output, so leaving the flag on for the window can't run away.
	harassTickFloor          = 10500
	harassMinHarvesterEvents = 2    // sustained pressure, not one-off
	pressureLookbackTicks    = 2000 // "recent" stress events window

	// A massing enemy near our base is a rush BEFORE it costs anything.
	//
	// Every other trigger here is damage already taken, so the flag can only
	// catch a rush that has both landed and landed early. Game 203 is what that
	// misses: enemy units near the base went from 2 to FIFTEEN at tick 8000, and
	// the first loss of any kind was an oil derrick at 11301, the first harvester
	// at 13301 and the first refinery at 13421 -- all past rushTickCutoff. So
	// is-rushed() was false for the whole game, build-base-defense-rush fired 0
	// times in 2559 evaluations, and every `not is-rushed()` gate stayed open
	// while fifteen units massed outside. Vimy lost its entire economy between
	// ticks 13000 and 16000 and the game at 26671.
	//
	// The function's own history is the same mistake once already: it read only
	// harvesters until game 179 lost its construction yard at 8760 with the flag
	// still false, and the fix added a third kind of DAMAGE rather than asking
	// whether damage is the right trigger.
	//
	// Six is above what scouting or a raiding pair looks like and below a real
	// army, and the radius matches NearBaseGroundUnits' own 0.20 of the map
	// diagonal so both sides of "near the base" mean the same thing.
	rushEnemyNearBase  = 6
	rushNearBaseRadius = 0.20
)

// enemiesMassingNearBase counts enemy COMBAT units within rushNearBaseRadius of
// any of our own buildings.
//
// Our buildings, not a centroid: a centroid of a base plus three captured oil
// derricks sits somewhere between them and is nowhere Vimy owns. Neutral tech
// structures are excluded for the same reason -- a derrick is a thing we hold,
// not part of the base we defend.
//
// Only what is currently VISIBLE, because the engine's enemy list is already
// fog-filtered. That is the right behaviour: a rush we cannot see is one we
// cannot answer either.
func enemiesMassingNearBase(gs *model.GameState) int {
	if gs == nil || len(gs.Enemies) == 0 {
		return 0
	}
	var base []model.Building
	for i := range gs.Buildings {
		if rules.IsNeutralTechStructure(gs.Buildings[i].Type) {
			continue
		}
		base = append(base, gs.Buildings[i])
	}
	if len(base) == 0 {
		return 0
	}
	mw, mh := float64(gs.MapWidth), float64(gs.MapHeight)
	reach := math.Sqrt(mw*mw+mh*mh) * rushNearBaseRadius
	reachSq := reach * reach

	n := 0
	for _, en := range gs.Enemies {
		if !rules.IsCombatUnit(en.Type) || rules.IsNeutralTechStructure(en.Type) {
			continue
		}
		for j := range base {
			dx := float64(en.X - base[j].X)
			dy := float64(en.Y - base[j].Y)
			if dx*dx+dy*dy <= reachSq {
				n++
				break
			}
		}
	}
	return n
}

// computePressureFlags returns (beingRushed, harvesterHarassed). Both can be
// false; the disjoint tick thresholds keep both from being true at once, though
// the prompt handles each independently.
func computePressureFlags(currentTick int, gs *model.GameState, stress []Event) (bool, bool) {
	if gs == nil {
		return false, false
	}

	harvesterAttackEvents := 0
	harvesterLostEvents := 0
	buildingLostEvents := 0
	for _, e := range stress {
		if currentTick-e.Tick > pressureLookbackTicks {
			continue
		}
		switch e.Kind {
		case EventHarvesterUnderAttack:
			harvesterAttackEvents++
		case EventHarvesterLost:
			harvesterLostEvents++
		case EventCriticalBuildingLost:
			buildingLostEvents++
		}
	}

	// A critical building lost inside the early window is a rush at least as
	// much as a harvester being shot at, and this function used to read only the
	// harvesters. Game 179 lost its construction yard at tick 8760 and its war
	// factory at 9180 - both inside the window - and the flag stayed false, so
	// build-base-defense-rush, whose whole purpose is the 200-credit floor a
	// base under attack needs, fired 0 times in 938 evaluations. Every other
	// `not is-rushed()` gate stayed open too, which is Vimy expanding while the
	// yard burns. EventCriticalBuildingLost was already in the stress feed this
	// function reads; nothing counted it.
	beingRushed := currentTick < rushTickCutoff &&
		(harvesterAttackEvents >= 1 || harvesterLostEvents >= 1 || buildingLostEvents >= 1 ||
			enemiesMassingNearBase(gs) >= rushEnemyNearBase)

	harvesterHarassed := currentTick >= harassTickFloor &&
		(harvesterAttackEvents >= harassMinHarvesterEvents || harvesterLostEvents >= 1)

	return beingRushed, harvesterHarassed
}
