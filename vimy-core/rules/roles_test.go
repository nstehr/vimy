package rules

import "testing"

// A doctrine naming the other side's units has them removed before anything
// reads the list — the selection code already skips them, but DoctrineParams
// reads raw names and moves rule priorities on the strength of one.
func TestFilterPreferencesDropsTheOtherSidesUnits(t *testing.T) {
	d := Doctrine{
		Name:              "Air Supremacy SEAD",
		PreferredVehicle:  []string{"v2_launcher", "medium_tank", "artillery"},
		PreferredInfantry: []string{"tanya", "flamethrower", "rocket_soldier"},
	}

	got, dropped := FilterPreferences(d, "england")
	if len(got.PreferredVehicle) != 2 || got.PreferredVehicle[0] != "medium_tank" {
		t.Errorf("vehicles = %v, want the V2 gone and artillery kept", got.PreferredVehicle)
	}
	if len(got.PreferredInfantry) != 2 || got.PreferredInfantry[0] != "tanya" {
		t.Errorf("infantry = %v, want the flamethrower gone", got.PreferredInfantry)
	}
	if len(dropped) != 2 {
		t.Errorf("dropped = %v, want v2_launcher and flamethrower", dropped)
	}
}

// The head of the list decides `siege-vehicle-first` and the radar-first build
// order, so dropping an unbuildable head has to promote the next real entry.
func TestFilterPreferencesPromotesTheNextBuildableHead(t *testing.T) {
	d := Doctrine{PreferredVehicle: []string{"v2_launcher", "artillery"}}
	got, _ := FilterPreferences(d, "france")
	if len(got.PreferredVehicle) == 0 || got.PreferredVehicle[0] != "artillery" {
		t.Fatalf("vehicles = %v, want artillery at the head", got.PreferredVehicle)
	}
	if DoctrineParams(got)["prefers-v2-launcher"] != 0 {
		t.Error("prefers-v2-launcher is still set for an Allied faction")
	}
	if DoctrineParams(got)["siege-vehicle-first"] != 1 {
		t.Error("siege-vehicle-first should hold: artillery is a siege vehicle it can build")
	}
}

func TestFilterPreferencesLeavesASovietListAlone(t *testing.T) {
	d := Doctrine{PreferredVehicle: []string{"v2_launcher", "heavy_tank", "medium_tank"}}
	got, dropped := FilterPreferences(d, "russia")
	if len(dropped) != 0 {
		t.Errorf("dropped %v from a Soviet list playing Soviet", dropped)
	}
	if len(got.PreferredVehicle) != 3 {
		t.Errorf("vehicles = %v, want all three kept", got.PreferredVehicle)
	}
}

// A role both sides build is never dropped, whatever the faction.
func TestFilterPreferencesKeepsSharedRoles(t *testing.T) {
	for _, faction := range []string{"russia", "england", "somethingelse"} {
		d := Doctrine{PreferredVehicle: []string{"medium_tank"}, PreferredInfantry: []string{"rocket_soldier", "engineer"}}
		got, dropped := FilterPreferences(d, faction)
		if len(dropped) != 0 {
			t.Errorf("%s: dropped %v, want none", faction, dropped)
		}
		if len(got.PreferredInfantry) != 2 {
			t.Errorf("%s: infantry = %v", faction, got.PreferredInfantry)
		}
	}
}

// The prompt lists rosters by side, so the side has to be stated rather than
// left to be inferred from a country name.
func TestUnbuildableRolesIsTheOtherSidesLockedRoster(t *testing.T) {
	allied := UnbuildableRoles("germany")
	if len(allied) == 0 {
		t.Fatal("germany can build everything?")
	}
	for _, role := range allied {
		if BuildableByFaction(role, "germany") {
			t.Errorf("%s listed as unbuildable but BuildableByFaction says otherwise", role)
		}
		if !BuildableByFaction(role, "russia") {
			t.Errorf("%s is unbuildable by both sides — it is not faction-locked", role)
		}
	}
	// The two the strategist kept naming for an Allied faction.
	var sawV2, sawFlak bool
	for _, role := range allied {
		sawV2 = sawV2 || role == "v2_launcher"
		sawFlak = sawFlak || role == "flak_truck"
	}
	if !sawV2 || !sawFlak {
		t.Errorf("germany's forbidden list = %v, want v2_launcher and flak_truck in it", allied)
	}
}

// Two of the three support powers are country-gated, not side-gated: a Soviet
// player who is not Russia still cannot call the Russian spy plane.
func TestSupportPowerReachableIsPerCountry(t *testing.T) {
	cases := []struct {
		power, faction string
		want           bool
	}{
		{"SovietParatroopers", "russia", true},
		{"SovietParatroopers", "ukraine", true},
		{"SovietParatroopers", "germany", false},
		{"SovietSpyPlane", "russia", true},
		{"SovietSpyPlane", "ukraine", false},
		{"SovietSpyPlane", "england", false},
		{"UkraineParabombs", "ukraine", true},
		{"UkraineParabombs", "russia", false},
		// Unknown powers are reachable: no evidence is not evidence of absence,
		// and a report that calls a merely-idle rule impossible is worse than
		// one that says nothing.
		{"NukePowerInfoOrder", "germany", true},
		{"GrantExternalConditionPowerInfoOrder", "england", true},
	}
	for _, c := range cases {
		if got := SupportPowerReachable(c.power, c.faction); got != c.want {
			t.Errorf("SupportPowerReachable(%q, %q) = %v, want %v", c.power, c.faction, got, c.want)
		}
	}
}
