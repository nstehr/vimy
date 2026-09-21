package rules

import (
	"reflect"
	"testing"
)

// The digest must see the rules people actually tune. Measured before this
// test existed: an all-zero doctrine compiles to 44 rules against a union of
// 112, so a digest taken against it alone was blind to 68 -- flee-harvesters,
// build-aa-defense, produce-extra-harvester among them. It would sit unchanged
// through a rule edit and report that nothing had happened.
func TestReferenceDoctrinesSpanTheParameterSpace(t *testing.T) {
	ds := referenceDoctrines()
	if len(ds) < 2 {
		t.Fatalf("got %d reference doctrines, want both ends of the range", len(ds))
	}

	var zeros, maxes int
	for _, d := range ds {
		v := reflect.ValueOf(d)
		var nonZero, atOne int
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Kind() != reflect.Float64 {
				continue
			}
			if f.Float() != 0 {
				nonZero++
			}
			if f.Float() == 1 {
				atOne++
			}
		}
		if nonZero == 0 {
			zeros++
		}
		if atOne > 0 && atOne == nonZero {
			maxes++
		}
	}
	if zeros == 0 {
		t.Error("no all-zero reference doctrine: rules gated on low values go unseen")
	}
	if maxes == 0 {
		t.Error("no all-max reference doctrine: rules gated on high values go unseen -- this was 68 of 112")
	}
}

// The references are the anchor for every digest ever recorded. Changing them
// silently re-dates the whole archive, so changing them has to be deliberate.
func TestReferenceDoctrinesAreStable(t *testing.T) {
	a, b := referenceDoctrines(), referenceDoctrines()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("referenceDoctrines is not deterministic")
	}
	for _, d := range a {
		if d.Name == "" {
			t.Error("a reference doctrine has no name; they are identified in logs by it")
		}
	}
}
