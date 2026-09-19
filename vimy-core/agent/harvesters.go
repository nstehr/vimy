package agent

import (
	"math"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
)

// Where a harvester's time actually goes.
//
// Income has sat near 1.7 credits a tick across every game measured, through a
// flee radius cut by two thirds, harvester counts from 9 to 11, refineries from
// 8 to 9, and an economy-first doctrine at economy_priority 0.8. Eleven
// harvesters against nine refineries should return far more than that, and
// three separate economy changes were made on inference about why they do not,
// because nothing in the archive could say.
//
// Guessing was the problem, so this counts rather than infers. It classifies
// each harvester at each sample and keeps the totals, which turns "the economy
// is bad" into one of four different findings with different fixes:
//
//	idle       the rules are not keeping them working
//	mining     they are working, and the ceiling is elsewhere
//	travelling ore near the base is gone and trips are long
//	atRefinery they are queueing to unload and the refineries are the bottleneck
//
// No new engine data. Position, the idle flag and the refineries the sidecar
// already sees are enough to separate all four, which keeps this on the right
// side of the observation-only rule.
type harvesterTracker struct {
	// Samples of one harvester in one state, so the four sum to the total time
	// harvesters existed rather than to the number of game ticks.
	Idle       int
	Mining     int
	Travelling int
	AtRefinery int

	// Distance to the nearest refinery, summed over samples where a harvester
	// was travelling, and the count of those samples. A rising average is ore
	// running out near the base — the thing no measure in the archive can
	// currently see.
	haulDistance float64
	haulSamples  int

	// Where each harvester was at the previous sample, to tell moving from
	// standing still. Cleared for harvesters that are gone.
	lastSeen map[int]position
}

type position struct{ x, y int }

// atRefineryCells is how close counts as "at the refinery".
//
// Two, not four, and the difference matters more than it looks. A harvester
// standing still is either unloading or mining, and the only thing separating
// them here is distance from a refinery — so the radius has to be smaller than
// the distance from a refinery to the ore. It was not:
//
//	game    mean haul   "at refinery"
//	 135    6.1 cells        14%
//	 136    3.9 cells        44%
//	 137    3.7 cells        39%
//	 138    4.4 cells        40%
//
// A perfect inverse correlation with haul distance, because at a radius of 4 a
// harvester MINING on ore 4 cells from a refinery was filed as docked. Three
// games of "harvesters are queueing" were mining, and the reading was used to
// argue against decoupling harvesters from refineries.
//
// Two cells is roughly the footprint of the 3x2 building itself, so it catches
// a harvester against the edge and little else. The residual limit is real and
// unfixable by geometry: where ore sits within two cells of a refinery the two
// states are genuinely indistinguishable from position alone. Read the mean
// haul alongside the split — when it approaches this radius, the split is not
// to be trusted.
const atRefineryCells = 2

func (h *harvesterTracker) reset() { *h = harvesterTracker{} }

// Samples is the total harvester-samples observed, the denominator for the
// four shares.
func (h *harvesterTracker) Samples() int {
	return h.Idle + h.Mining + h.Travelling + h.AtRefinery
}

// MeanHaulDistance is how far from a refinery a harvester was while moving.
// Zero when nothing was ever observed travelling.
func (h *harvesterTracker) MeanHaulDistance() float64 {
	if h.haulSamples == 0 {
		return 0
	}
	return h.haulDistance / float64(h.haulSamples)
}

func (h *harvesterTracker) observe(gs model.GameState) {
	var refineries []position
	for _, b := range gs.Buildings {
		if rules.IsRefinery(b.Type) {
			refineries = append(refineries, position{b.X, b.Y})
		}
	}

	seen := make(map[int]position)
	for _, u := range gs.Units {
		if !rules.IsHarvester(u.Type) {
			continue
		}
		here := position{u.X, u.Y}
		prev, hadPrev := h.lastSeen[u.ID]
		seen[u.ID] = here
		if !hadPrev {
			// Nothing to compare against yet, so moving cannot be told from
			// standing still. Counting it anyway would silently credit every
			// new harvester's first sample to mining.
			continue
		}

		dist, near := nearestRefinery(here, refineries)

		switch {
		case u.Idle:
			h.Idle++
		case prev != here:
			// Moving. Its distance from a refinery is the trip length we cannot
			// otherwise see.
			h.Travelling++
			if near {
				h.haulDistance += dist
				h.haulSamples++
			}
		case near && dist <= atRefineryCells:
			// Standing still against a refinery: unloading, or waiting to.
			h.AtRefinery++
		default:
			// Standing still away from any refinery, and not idle. That is a
			// full harvester cycle's productive half.
			h.Mining++
		}
	}
	// A harvester that is gone must not leave a stale position behind for an id
	// the engine may reuse.
	h.lastSeen = seen
}

func nearestRefinery(p position, refineries []position) (float64, bool) {
	if len(refineries) == 0 {
		return 0, false
	}
	best := math.MaxFloat64
	for _, r := range refineries {
		dx := float64(p.x - r.x)
		dy := float64(p.y - r.y)
		if d := dx*dx + dy*dy; d < best {
			best = d
		}
	}
	return math.Sqrt(best), true
}
