package main

import (
	"sort"
	"strings"

	"github.com/nstehr/vimy/vimy-core/rules"
)

// What the doctrine was trying to do, so the report stops reporting obedience
// as failure.
//
// Game 95 ran an armoured directive with air_weight at 0.05, and Currie's two
// leading findings were `queue-ready(Aircraft)` blocking 99% of its chances and
// `build-airfield` first acting at 73% of the game. Both are the rule set doing
// exactly what it was told. A blame count cannot tell a gate that is stuck from
// a gate the doctrine deliberately closed, because it never sees the doctrine.

// axisFloor is the weight below which an axis counts as switched off. The
// strategist expresses "not this game" as a small number rather than a zero —
// 0.05 for air in game 95 — so this has to be a little above zero, and low
// enough that a real but minor investment is not written off.
const axisFloor = 0.12

// axisOf names the doctrine axis a rule serves, or "" when it serves none.
//
// Matched on the rule's own name rather than its category: `build-airfield` is
// in the `economy` category and `produce-aircraft` is in `produce-aircraft`,
// yet both live or die by air_weight.
func axisOf(rule string) string {
	switch {
	case strings.Contains(rule, "aircraft"), strings.Contains(rule, "airfield"),
		strings.Contains(rule, "air-attack"), strings.Contains(rule, "helipad"),
		strings.Contains(rule, "-air"), strings.HasPrefix(rule, "air-"):
		return "air"
	case strings.Contains(rule, "naval"), strings.Contains(rule, "ship"),
		strings.Contains(rule, "submarine"), strings.Contains(rule, "cruiser"):
		return "naval"
	case strings.Contains(rule, "infantry"), strings.Contains(rule, "rifleman"),
		strings.Contains(rule, "rocket-soldier"), strings.Contains(rule, "specialist"):
		return "infantry"
	case strings.Contains(rule, "vehicle"), strings.Contains(rule, "tank"),
		strings.Contains(rule, "war-factory"):
		return "vehicle"
	case strings.Contains(rule, "capture"), strings.Contains(rule, "engineer"):
		return "capture"
	case strings.Contains(rule, "superweapon"), strings.Contains(rule, "nuke"),
		strings.Contains(rule, "iron-curtain"), strings.Contains(rule, "missile-silo"):
		return "superweapon"
	}
	return ""
}

// quietAxes reports the axes the doctrine kept near zero for most of the game,
// with the median weight it chose.
//
// Median rather than mean: the strategist swings a knob hard between windows,
// so one spike should not make an axis look invested in.
func quietAxes(doctrines []rules.Doctrine) map[string]float64 {
	if len(doctrines) == 0 {
		return nil
	}
	series := map[string][]float64{}
	for _, d := range doctrines {
		series["air"] = append(series["air"], d.AirWeight)
		series["naval"] = append(series["naval"], d.NavalWeight)
		series["infantry"] = append(series["infantry"], d.InfantryWeight)
		series["vehicle"] = append(series["vehicle"], d.VehicleWeight)
		series["capture"] = append(series["capture"], d.CapturePriority)
		series["superweapon"] = append(series["superweapon"], d.SuperweaponPriority)
	}
	quiet := map[string]float64{}
	for axis, vs := range series {
		sort.Float64s(vs)
		med := vs[len(vs)/2]
		if med < axisFloor {
			quiet[axis] = med
		}
	}
	return quiet
}
