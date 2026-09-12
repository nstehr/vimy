package agent

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// The whole value of this tracker is the distinction it refuses to blur. An
// enemy that vanishes where we are still standing died; one that vanishes in
// the dark may have walked away, and reporting it as a kill would tell the
// strategist it is winning fights it is losing.
func TestKillTrackerSeparatesWatchedDeathsFromFog(t *testing.T) {
	base := model.GameState{
		MapWidth:  1000,
		MapHeight: 1000,
		// Our only eyes are here.
		Units: []model.Unit{{ID: 1, Type: "e1", X: 500, Y: 500}},
	}

	seen := base
	seen.Enemies = []model.Enemy{
		{ID: 90, Type: "e1", X: 505, Y: 505},   // under our nose
		{ID: 91, Type: "3tnk", X: 900, Y: 900}, // far away, seen once
		{ID: 92, Type: "proc", X: 510, Y: 510}, // a building under our nose
	}

	var k killTracker
	k.observe(seen)

	gone := base // all three enemies no longer visible
	k.observe(gone)

	if k.Units != 1 {
		t.Errorf("watched unit deaths = %d, want 1 (the one beside our rifleman)", k.Units)
	}
	if k.Buildings != 1 {
		t.Errorf("watched building deaths = %d, want 1", k.Buildings)
	}
	if k.PresumedUnits != 1 {
		t.Errorf("presumed unit deaths = %d, want 1 (the tank that vanished across the map)", k.PresumedUnits)
	}
	if k.PresumedBuildings != 0 {
		t.Errorf("presumed building deaths = %d, want 0", k.PresumedBuildings)
	}
}

// A unit still on screen is not a kill, however long we watch it.
func TestKillTrackerDoesNotCountTheLiving(t *testing.T) {
	gs := model.GameState{
		MapWidth: 1000, MapHeight: 1000,
		Units:   []model.Unit{{ID: 1, Type: "e1", X: 500, Y: 500}},
		Enemies: []model.Enemy{{ID: 90, Type: "e1", X: 505, Y: 505}},
	}
	var k killTracker
	for range 5 {
		k.observe(gs)
	}
	if k.Units != 0 || k.PresumedUnits != 0 {
		t.Errorf("counted %d killed and %d presumed against an enemy that never left", k.Units, k.PresumedUnits)
	}
}

// Buildings watch their own ground: a refinery is an observer even with no
// units nearby.
func TestKillTrackerCountsBuildingsAsEyes(t *testing.T) {
	gs := model.GameState{
		MapWidth: 1000, MapHeight: 1000,
		Buildings: []model.Building{{ID: 1, Type: "proc", X: 200, Y: 200}},
		Enemies:   []model.Enemy{{ID: 90, Type: "e1", X: 205, Y: 205}},
	}
	var k killTracker
	k.observe(gs)

	gs.Enemies = nil
	k.observe(gs)

	if k.Units != 1 {
		t.Errorf("watched deaths = %d, want 1: the refinery saw it", k.Units)
	}
}
