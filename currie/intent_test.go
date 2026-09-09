package main

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/rules"
)

// The classifier decides which findings get hidden, so over-matching is worse
// than under-matching: a rule wrongly tied to a quiet axis disappears from the
// report entirely.
func TestAxisOfDoesNotOverMatch(t *testing.T) {
	cases := map[string]string{
		"build-airfield":    "air",
		"produce-aircraft":  "air",
		"squad-air-attack":  "air",
		"build-naval-yard":  "naval",
		"produce-infantry":  "infantry",
		"produce-vehicle":   "vehicle",
		"build-war-factory": "vehicle",
		"produce-mad-tank":  "vehicle",
		"capture-building":  "capture",
		"fire-nuke":         "superweapon",
		// Everything else must stay unclassified, or a quiet axis would hide it.
		"repair-buildings":        "",
		"build-refinery":          "",
		"produce-extra-harvester": "",
		"squad-attack-known-base": "",
		"build-base-defense":      "",
		"build-aa-defense":        "",
		"scout-with-idle-units":   "",
		"deploy-mcv":              "",
	}
	for rule, want := range cases {
		if got := axisOf(rule); got != want {
			t.Errorf("axisOf(%q) = %q, want %q", rule, got, want)
		}
	}
}

// An axis is quiet only if it stayed low for most of the game. The strategist
// swings a knob hard between windows, so one spike must not make an axis look
// invested in, and one dip must not switch a live axis off.
func TestQuietAxesUsesTheMedian(t *testing.T) {
	ds := []rules.Doctrine{
		{AirWeight: 0.05, VehicleWeight: 0.20, NavalWeight: 0},
		{AirWeight: 0.05, VehicleWeight: 0.70, NavalWeight: 0},
		{AirWeight: 0.90, VehicleWeight: 0.65, NavalWeight: 0}, // one air spike
		{AirWeight: 0.05, VehicleWeight: 0.60, NavalWeight: 0},
		{AirWeight: 0.05, VehicleWeight: 0.55, NavalWeight: 0},
	}
	quiet := quietAxes(ds)
	if _, ok := quiet["air"]; !ok {
		t.Error("air held at 0.05 in four of five windows should read as quiet")
	}
	if _, ok := quiet["naval"]; !ok {
		t.Error("naval at zero throughout should read as quiet")
	}
	if _, ok := quiet["vehicle"]; ok {
		t.Errorf("vehicle with a median of 0.60 should not read as quiet: %v", quiet)
	}
	if len(quietAxes(nil)) != 0 {
		t.Error("no doctrines should yield no quiet axes, not a panic")
	}
}
