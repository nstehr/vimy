package rules

import (
	"reflect"
	"sort"
	"testing"
)

// Every action a compiled rule can run must be nameable, or vimyc cannot emit a
// rule that runs it.
//
// Parameterised actions are the exception: `FormSquad("ground-attack", ...)`
// returns a closure, and a closure per doctrine cannot be a fixed id. Those are
// named by their factory instead, which is what `do` taking arguments is for.
func TestEveryRuleActionIsRegistered(t *testing.T) {
	byPtr := map[uintptr]bool{}
	for _, fn := range ActionRegistry {
		byPtr[reflect.ValueOf(fn).Pointer()] = true
	}

	real, err := RealDoctrines()
	if err != nil {
		t.Fatal(err)
	}
	unresolved := map[string]bool{}
	for _, d := range append(real, DefaultDoctrine()) {
		for _, r := range CompileDoctrine(d) {
			if !byPtr[reflect.ValueOf(r.Action).Pointer()] {
				unresolved[r.Name] = true
			}
		}
	}
	for _, r := range DefaultRules() {
		if !byPtr[reflect.ValueOf(r.Action).Pointer()] {
			unresolved[r.Name] = true
		}
	}

	expected := map[string]bool{}
	for _, n := range parameterisedActionRules {
		expected[n] = true
	}

	var surprise, gone []string
	for n := range unresolved {
		if !expected[n] {
			surprise = append(surprise, n)
		}
	}
	for n := range expected {
		if !unresolved[n] {
			gone = append(gone, n)
		}
	}
	sort.Strings(surprise)
	sort.Strings(gone)

	if len(surprise) > 0 {
		t.Errorf("these run an action no id names, and are not parameterised: %v", surprise)
	}
	// Kept honest in both directions: a rule that stops being parameterised
	// should drop off the list rather than sit there excusing nothing.
	if len(gone) > 0 {
		t.Errorf("these are listed as parameterised but now resolve: %v", gone)
	}
}

// Rules whose action is built by a factory — FormSquad, SquadDefend and the
// nine others in actions.go — so it carries arguments and cannot be a fixed id.
var parameterisedActionRules = []string{
	"clear-healed-units",
	"flee-harvesters",
	"form-air-attack",
	"form-defense-squad",
	"form-ground-attack",
	"form-naval-attack",
	"recall-overextended-ground-attack",
	"recall-overextended-naval-attack",
	"retreat-damaged-units",
	"squad-air-attack",
	"squad-air-attack-known-base",
	"squad-air-reengage",
	"squad-attack",
	"squad-attack-known-base",
	"squad-defend-base",
	"squad-disengage-ground-attack",
	"squad-disengage-naval-attack",
	"squad-focus-fire",
	"squad-naval-attack",
	"squad-naval-attack-known-base",
	"squad-naval-reengage",
	"squad-reengage",
}
