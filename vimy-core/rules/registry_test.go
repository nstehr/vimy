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

	// The seed rules only. This walked every rule CompileDoctrine could emit
	// until that compiler was deleted; a vimyc rule set names its actions as
	// text, and that they resolve is checked by
	// `TestEveryActionInTheArtifactResolves`, which is a stronger test of the
	// same thing — a name that is not in the registry fails to load at all.
	unresolved := map[string]bool{}
	for _, r := range DefaultRules() {
		if !byPtr[reflect.ValueOf(r.Action).Pointer()] {
			unresolved[r.Name] = true
		}
	}

	// The seed rules use no factory-built action, so every one of them must be
	// in the registry. The rules that do take arguments arrive from vimyc as
	// text and are resolved by `resolveAction`, where an unregistered name
	// fails the whole load rather than passing quietly.
	expected := map[string]bool{}

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
