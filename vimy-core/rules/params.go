package rules

import (
	"reflect"
	"strings"
)

// The doctrine as vimyc sees it.
//
// Not the Doctrine struct: a rule set reads numbers, and the ordered []string
// preference lists are consumed by SetPreferences. What the compiler actually
// asks of those lists is a handful of yes/no questions, so they cross as 0 or 1
// and the lists stay here.

// DoctrineParams renders a doctrine as the numbers a vimyc rule set declares.
//
// Numeric fields come from their JSON tags, which differ from vimyc's spelling
// only in the separator, so adding a knob to Doctrine makes it available to a
// rule set without touching this.
func DoctrineParams(d Doctrine) map[string]float64 {
	out := map[string]float64{}
	v := reflect.ValueOf(d)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.ReplaceAll(tag, "_", "-")
		switch v.Field(i).Kind() {
		case reflect.Float64:
			out[name] = v.Field(i).Float()
		case reflect.Int:
			out[name] = float64(v.Field(i).Int())
		}
	}

	c := &doctrineCompiler{d: d}
	for name, yes := range map[string]bool{
		"prefers-radar-gated-primary": prefersRadarGatedPrimary(d.PreferredVehicle),
		"prefers-v2-launcher":         c.prefersVehicle("v2_launcher"),
		"prefers-artillery":           c.prefersVehicle("artillery"),
		"prefers-shock-trooper":       c.prefersInfantry("shock_trooper"),
		"prefers-flamethrower":        c.prefersInfantry("flamethrower"),
		"specialist-infantry-first":   headIsOneOf(d.PreferredInfantry, specialistInfantryRoles...),
		"siege-vehicle-first":         headIsOneOf(d.PreferredVehicle, "v2_launcher", "artillery"),
		"tech-naval-first":            headIsOneOf(d.PreferredNaval, "missile_sub", "cruiser", "destroyer"),
	} {
		if yes {
			out[name] = 1
		} else {
			out[name] = 0
		}
	}
	return out
}

// headIsOneOf reports whether the list's first entry is one of `want` — the
// "this is the plan" test the compiler applies to the ordered preference lists.
func headIsOneOf(list []string, want ...string) bool {
	if len(list) == 0 {
		return false
	}
	for _, w := range want {
		if list[0] == w {
			return true
		}
	}
	return false
}
