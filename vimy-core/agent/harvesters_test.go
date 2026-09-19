package agent

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func state(refineryAt []model.Building, units ...model.Unit) model.GameState {
	return model.GameState{Buildings: refineryAt, Units: units}
}

var oneRefinery = []model.Building{{ID: 1, Type: "proc", X: 50, Y: 50}}

// The first sighting of a harvester cannot be classified: with no previous
// position, moving is indistinguishable from standing still. Counting it would
// silently credit every new harvester to mining, and harvesters are rebuilt
// constantly — 90 acts in game 130.
func TestFirstSightingIsNotCounted(t *testing.T) {
	var h harvesterTracker
	h.observe(state(oneRefinery, model.Unit{ID: 7, Type: "harv", X: 80, Y: 80}))
	if got := h.Samples(); got != 0 {
		t.Fatalf("samples = %d, want 0 on first sighting", got)
	}
	h.observe(state(oneRefinery, model.Unit{ID: 7, Type: "harv", X: 80, Y: 80}))
	if h.Samples() != 1 {
		t.Fatalf("samples = %d, want 1 on the second", h.Samples())
	}
}

func TestPhasesAreDistinguished(t *testing.T) {
	cases := []struct {
		name        string
		first, then model.Unit
		want        func(h harvesterTracker) int
		phase       string
	}{
		{
			name:  "standing away from a refinery is mining",
			first: model.Unit{ID: 1, Type: "harv", X: 90, Y: 90},
			then:  model.Unit{ID: 1, Type: "harv", X: 90, Y: 90},
			want:  func(h harvesterTracker) int { return h.Mining },
			phase: "mining",
		},
		{
			name:  "changing position is travelling",
			first: model.Unit{ID: 1, Type: "harv", X: 90, Y: 90},
			then:  model.Unit{ID: 1, Type: "harv", X: 88, Y: 90},
			want:  func(h harvesterTracker) int { return h.Travelling },
			phase: "travelling",
		},
		{
			// Against the building's edge. 52,51 was here before and is 2.24
			// cells out — ore range once the radius tightened to 2, which is
			// the whole point of that change.
			name:  "standing against a refinery is unloading or queued",
			first: model.Unit{ID: 1, Type: "harv", X: 51, Y: 51},
			then:  model.Unit{ID: 1, Type: "harv", X: 51, Y: 51},
			want:  func(h harvesterTracker) int { return h.AtRefinery },
			phase: "at refinery",
		},
		{
			// Idle wins over position: a harvester parked on top of a refinery
			// doing nothing is idle, not unloading.
			name:  "the idle flag wins over position",
			first: model.Unit{ID: 1, Type: "harv", X: 50, Y: 50},
			then:  model.Unit{ID: 1, Type: "harv", X: 50, Y: 50, Idle: true},
			want:  func(h harvesterTracker) int { return h.Idle },
			phase: "idle",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var h harvesterTracker
			h.observe(state(oneRefinery, c.first))
			h.observe(state(oneRefinery, c.then))
			if got := c.want(h); got != 1 {
				t.Errorf("%s = %d, want 1 (tracker: idle %d mining %d travel %d refinery %d)",
					c.phase, got, h.Idle, h.Mining, h.Travelling, h.AtRefinery)
			}
			if h.Samples() != 1 {
				t.Errorf("samples = %d, want exactly 1", h.Samples())
			}
		})
	}
}

// Only harvesters. An APC standing next to a refinery is not economic data.
func TestOtherUnitsAreIgnored(t *testing.T) {
	var h harvesterTracker
	for range 2 {
		h.observe(state(oneRefinery,
			model.Unit{ID: 2, Type: "apc", X: 50, Y: 50},
			model.Unit{ID: 3, Type: "2tnk", X: 90, Y: 90}))
	}
	if h.Samples() != 0 {
		t.Errorf("samples = %d, want 0", h.Samples())
	}
}

// The haul distance is what says whether ore near the base has run out, so it
// must average only the samples where a harvester was actually moving.
func TestHaulDistanceAveragesOnlyTravel(t *testing.T) {
	var h harvesterTracker
	// Sitting still at 30 cells away: mining, and must not enter the average.
	h.observe(state(oneRefinery, model.Unit{ID: 1, Type: "harv", X: 80, Y: 50}))
	h.observe(state(oneRefinery, model.Unit{ID: 1, Type: "harv", X: 80, Y: 50}))
	if d := h.MeanHaulDistance(); d != 0 {
		t.Fatalf("mean haul = %.1f after no travel, want 0", d)
	}
	// Now moving, 10 cells out.
	h.observe(state(oneRefinery, model.Unit{ID: 1, Type: "harv", X: 60, Y: 50}))
	if d := h.MeanHaulDistance(); d != 10 {
		t.Errorf("mean haul = %.1f, want 10", d)
	}
}

// A harvester that dies must not leave its position behind: the engine reuses
// actor ids, and a stale entry would compare a new harvester against a dead
// one's position and read as travelling.
func TestDeadHarvestersDropTheirPosition(t *testing.T) {
	var h harvesterTracker
	h.observe(state(oneRefinery, model.Unit{ID: 1, Type: "harv", X: 90, Y: 90}))
	h.observe(state(oneRefinery)) // it dies
	if _, stale := h.lastSeen[1]; stale {
		t.Fatal("dead harvester left a position behind")
	}
	// A new harvester reusing id 1 elsewhere is a first sighting, not a move.
	before := h.Samples()
	h.observe(state(oneRefinery, model.Unit{ID: 1, Type: "harv", X: 10, Y: 10}))
	if h.Samples() != before {
		t.Errorf("samples = %d, want %d: reused id counted as a move", h.Samples(), before)
	}
}

// With no refinery standing there is nothing to measure distance from, and the
// tracker must still classify rather than divide by a missing building.
func TestNoRefineryStillClassifies(t *testing.T) {
	var h harvesterTracker
	h.observe(state(nil, model.Unit{ID: 1, Type: "harv", X: 90, Y: 90}))
	h.observe(state(nil, model.Unit{ID: 1, Type: "harv", X: 90, Y: 90}))
	if h.Mining != 1 {
		t.Errorf("mining = %d, want 1 with no refinery on the map", h.Mining)
	}
	if h.MeanHaulDistance() != 0 {
		t.Errorf("mean haul = %.1f, want 0", h.MeanHaulDistance())
	}
}

// The refinery radius must be smaller than the distance from a refinery to the
// ore, or mining is filed as docking. It was not: at a radius of 4, games with
// hauls of 3.7 to 4.4 cells reported 39-44% "at refinery" against game 135's
// 14% on a 6.1-cell haul — a perfect inverse correlation with haul distance,
// and three games of "the refineries are a bottleneck" that were mining.
func TestMiningNearARefineryIsNotDocking(t *testing.T) {
	var h harvesterTracker
	// Ore four cells from the refinery: the old radius swallowed this.
	at := model.Unit{ID: 1, Type: "harv", X: 54, Y: 50}
	h.observe(state(oneRefinery, at))
	h.observe(state(oneRefinery, at))

	if h.Mining != 1 {
		t.Errorf("mining = %d, want 1: a harvester four cells out is on ore, not docked", h.Mining)
	}
	if h.AtRefinery != 0 {
		t.Errorf("atRefinery = %d, want 0", h.AtRefinery)
	}
}

// Still has to catch a harvester actually against the building.
func TestDockedHarvesterIsStillDetected(t *testing.T) {
	var h harvesterTracker
	at := model.Unit{ID: 1, Type: "harv", X: 51, Y: 51}
	h.observe(state(oneRefinery, at))
	h.observe(state(oneRefinery, at))

	if h.AtRefinery != 1 {
		t.Errorf("atRefinery = %d, want 1: this one is on the building's edge", h.AtRefinery)
	}
}
