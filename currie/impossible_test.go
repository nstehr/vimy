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
