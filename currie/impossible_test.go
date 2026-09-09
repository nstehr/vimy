package main

import "testing"

func TestImpossibleForDetects(t *testing.T) {
	cases := []struct{ src, faction, want string }{
		{"support-power-ready(SovietParatroopers)", "germany", "SovietParatroopers"},
		{"support-power-ready(UkraineParabombs)", "germany", "UkraineParabombs"},
		{"support-power-ready(SovietSpyPlane)", "ukraine", "SovietSpyPlane"},
		{"support-power-ready(SovietParatroopers)", "russia", ""},
		{"has-role(v2-launcher)", "germany", "v2-launcher"},
		{"has-role(artillery)", "germany", ""},
		{"not has-role(iron-curtain)", "germany", ""},
		{"support-power-ready(NukePowerInfoOrder)", "germany", ""},
	}
	for _, c := range cases {
		if got := impossibleFor(c.src, c.faction); got != c.want {
			t.Errorf("impossibleFor(%q, %q) = %q, want %q", c.src, c.faction, got, c.want)
		}
	}
}

// Completion guards are success, not blame. `not squad-exists(air-attack)`
// blocking every state means the squad exists and its forming rule correctly
// declined to run twice — but fed to the model it comes back as a top finding.
func TestSatisfiedByCompletionCoversSquadsAndIntel(t *testing.T) {
	guards := []string{
		"require (not squad-exists(air-attack) and count(unassigned-idle-air) >= 2)",
		"require not has-role(construction-yard)",
		"require not has-unit(mcv)",
		"require not has-enemy-intel()",
		"require role-count(radar) == 0",
	}
	for _, g := range guards {
		if !satisfiedByCompletion(g) {
			t.Errorf("%q should read as a completion guard", g)
		}
	}
	// Real constraints must not be swept up with them.
	real := []string{
		"require not queue-busy(Vehicle)",
		"require cash >= cost",
		"require not has-retreating-units()",
		"require count(idle-scouts) > 0",
		"require not enemies-visible",
	}
	for _, r := range real {
		if satisfiedByCompletion(r) {
			t.Errorf("%q is a real constraint, not a completion guard", r)
		}
	}
}
