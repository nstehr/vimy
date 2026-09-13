package agent

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// Forward and at home imply opposite fixes, so the split is the whole point of
// the tracker and the thing worth pinning.
func TestLossTrackerSplitsForwardFromHome(t *testing.T) {
	// Base at (500,500) on a 1000x1000 map: home reaches ~283.
	base := []model.Building{{ID: 1, Type: "fact", X: 500, Y: 500}}
	gs := model.GameState{
		MapWidth: 1000, MapHeight: 1000, Buildings: base,
		Units: []model.Unit{
			{ID: 10, Type: "2tnk", X: 520, Y: 520}, // at home
			{ID: 11, Type: "2tnk", X: 950, Y: 950}, // deep in their half
			{ID: 12, Type: "e1", X: 940, Y: 940},   // forward with it
			{ID: 13, Type: "e1", X: 505, Y: 505},   // holding the base
		},
	}

	var l lossTracker
	l.observe(gs)

	gs.Units = nil // everything dies where it stood
	l.observe(gs)

	if l.Forward["vehicle"] != 1 {
		t.Errorf("forward vehicle losses = %d, want 1", l.Forward["vehicle"])
	}
	if l.NearBase["vehicle"] != 1 {
		t.Errorf("home vehicle losses = %d, want 1", l.NearBase["vehicle"])
	}
	if l.Forward["infantry"] != 1 || l.NearBase["infantry"] != 1 {
		t.Errorf("infantry split = forward %d / home %d, want 1 / 1",
			l.Forward["infantry"], l.NearBase["infantry"])
	}
}

// A unit that is still alive is not a loss, however far out it has wandered.
func TestLossTrackerIgnoresTheLiving(t *testing.T) {
	gs := model.GameState{
		MapWidth: 1000, MapHeight: 1000,
		Buildings: []model.Building{{ID: 1, Type: "fact", X: 500, Y: 500}},
		Units:     []model.Unit{{ID: 10, Type: "2tnk", X: 950, Y: 950}},
	}
	var l lossTracker
	for range 4 {
		l.observe(gs)
	}
	if l.Forward["vehicle"] != 0 || l.NearBase["vehicle"] != 0 {
		t.Errorf("counted a living unit: forward %d home %d", l.Forward["vehicle"], l.NearBase["vehicle"])
	}
}
