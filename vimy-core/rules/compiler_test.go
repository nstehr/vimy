package rules

import (
	"strings"
	"testing"

	"github.com/expr-lang/expr"
)

func TestCompileDoctrineBalanced(t *testing.T) {
	d := DefaultDoctrine()
	rules := CompileDoctrine(d)

	if len(rules) == 0 {
		t.Fatal("CompileDoctrine returned no rules")
	}

	// Verify all rules compile with expr
	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	// Check core rules are present
	coreNames := map[string]bool{
		"deploy-mcv":              false,
		"place-ready-building":    false,
		"place-ready-defense":     false,
		"cancel-stuck-aircraft":   false,
		"repair-buildings":        false,
		"return-idle-harvesters":  false,
		"capture-building":        false,
		"produce-engineer":        false,
	}
	for _, r := range rules {
		if _, ok := coreNames[r.Name]; ok {
			coreNames[r.Name] = true
		}
	}
	for name, found := range coreNames {
		if !found {
			t.Errorf("core rule %q missing from compiled doctrine", name)
		}
	}
}

func TestCompileDoctrineAggressive(t *testing.T) {
	d := Doctrine{
		Name:                  "Blitzkrieg",
		EconomyPriority:       0.3,
		Aggression:            0.9,
		GroundDefensePriority: 0.0,
		AirDefensePriority:    0.0,
		InfantryWeight:        0.6,
		VehicleWeight:         0.8,
		AirWeight:             0.0,
		NavalWeight:           0.0,
		GroundAttackGroupSize: 4,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.5,
	}
	rules := CompileDoctrine(d)

	// Verify all rules compile
	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v", r.Name, err)
		}
	}

	// Air and naval building/production rules should be absent (weight=0.0)
	for _, r := range rules {
		if r.Name == "build-airfield" || r.Name == "produce-aircraft" {
			t.Errorf("unexpected air rule %q when AirWeight=0", r.Name)
		}
		if r.Name == "build-naval-yard" || r.Name == "produce-ship" {
			t.Errorf("unexpected naval rule %q when NavalWeight=0", r.Name)
		}
	}

	// Infantry and vehicle rules should be present
	found := map[string]bool{}
	for _, r := range rules {
		found[r.Name] = true
	}
	if !found["build-barracks"] {
		t.Error("expected build-barracks with InfantryWeight=0.6")
	}
	if !found["build-war-factory"] {
		t.Error("expected build-war-factory with VehicleWeight=0.8")
	}
	if !found["produce-infantry"] {
		t.Error("expected produce-infantry with InfantryWeight=0.6")
	}
	if !found["produce-vehicle"] {
		t.Error("expected produce-vehicle with VehicleWeight=0.8")
	}
}

func TestCompileDoctrineEconomyOnly(t *testing.T) {
	d := Doctrine{
		Name:                  "Turtle",
		EconomyPriority:       0.9,
		Aggression:            0.1,
		GroundDefensePriority: 0.0,
		AirDefensePriority:    0.0,
		InfantryWeight:        0.0,
		VehicleWeight:         0.0,
		AirWeight:             0.0,
		NavalWeight:           0.0,
		GroundAttackGroupSize: 12,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.3,
	}
	rules := CompileDoctrine(d)

	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v", r.Name, err)
		}
	}

	// No military building or production rules when all weights are 0
	for _, r := range rules {
		switch r.Name {
		case "build-barracks", "build-war-factory", "build-airfield", "build-naval-yard",
			"produce-infantry", "produce-vehicle", "produce-aircraft", "produce-ship",
			"build-missile-silo", "build-iron-curtain",
			"fire-nuke", "fire-iron-curtain",
			"fire-spy-plane", "fire-spy-plane-update", "fire-paratroopers", "fire-parabombs":
			t.Errorf("unexpected military rule %q when all unit weights=0", r.Name)
		}
	}
}

func TestCompileDoctrineFullSpectrum(t *testing.T) {
	d := Doctrine{
		Name:                      "Full Spectrum",
		EconomyPriority:           0.5,
		Aggression:                0.5,
		GroundDefensePriority:     0.5,
		AirDefensePriority:        0.5,
		InfantryWeight:            0.5,
		VehicleWeight:             0.5,
		AirWeight:                 0.5,
		NavalWeight:               0.5,
		GroundAttackGroupSize:     6,
		AirAttackGroupSize:        2,
		NavalAttackGroupSize:      3,
		ScoutPriority:             0.5,
		SpecializedInfantryWeight: 0.5,
		SuperweaponPriority:       0.5,
	}
	rules := CompileDoctrine(d)

	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v", r.Name, err)
		}
	}

	// All building and production types should be present
	expected := []string{
		"build-barracks", "build-war-factory", "build-airfield", "build-naval-yard",
		"produce-infantry", "produce-vehicle", "produce-aircraft", "produce-ship",
		"produce-specialist-infantry",
		"form-air-attack", "squad-air-attack", "squad-air-attack-known-base",
		"form-naval-attack", "squad-naval-attack",
		"build-base-defense", "build-aa-defense",
		"build-missile-silo", "build-iron-curtain",
		"fire-nuke", "fire-iron-curtain",
		"fire-spy-plane", "fire-paratroopers", "fire-parabombs",
	}
	found := map[string]bool{}
	for _, r := range rules {
		found[r.Name] = true
	}
	for _, name := range expected {
		if !found[name] {
			t.Errorf("expected rule %q in full spectrum doctrine", name)
		}
	}
}

func TestCompileDoctrineRuleCount(t *testing.T) {
	// More unit types enabled → more rules
	minimal := CompileDoctrine(Doctrine{GroundAttackGroupSize: 5, AirAttackGroupSize: 2, NavalAttackGroupSize: 3})
	full := CompileDoctrine(Doctrine{
		EconomyPriority:       0.5,
		TechPriority:          0.5,
		GroundDefensePriority: 0.5,
		AirDefensePriority:    0.5,
		InfantryWeight:        0.5,
		VehicleWeight:         0.5,
		AirWeight:             0.5,
		NavalWeight:           0.5,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.5,
	})

	if len(full) <= len(minimal) {
		t.Errorf("full spectrum (%d rules) should have more rules than minimal (%d rules)",
			len(full), len(minimal))
	}
}

func TestCompileDoctrineHighDefense(t *testing.T) {
	d := Doctrine{
		Name:                  "Turtle Defense",
		EconomyPriority:       0.8,
		GroundDefensePriority: 0.9,
		AirDefensePriority:    0.9,
		InfantryWeight:        0.3,
		VehicleWeight:         0.3,
		GroundAttackGroupSize: 8,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.3,
	}
	rules := CompileDoctrine(d)

	// Verify all rules compile with expr
	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	// Defense structure rules should be present
	found := map[string]bool{}
	for _, r := range rules {
		found[r.Name] = true
	}
	if !found["build-base-defense"] {
		t.Error("expected build-base-defense with DefensePriority=0.9")
	}
	if !found["build-aa-defense"] {
		t.Error("expected build-aa-defense with DefensePriority=0.9")
	}
	if !found["place-ready-defense"] {
		t.Error("expected place-ready-defense as core rule")
	}

	// Defense rules should use defense category, not economy
	for _, r := range rules {
		if r.Name == "build-base-defense" || r.Name == "build-aa-defense" || r.Name == "place-ready-defense" {
			if r.Category != "defense" {
				t.Errorf("rule %q should have category 'defense', got %q", r.Name, r.Category)
			}
		}
	}

	// Economy scaling rules should be present
	if !found["build-advanced-power"] {
		t.Error("expected build-advanced-power with EconomyPriority=0.8")
	}
	if !found["build-ore-silo"] {
		t.Error("expected build-ore-silo with EconomyPriority=0.8")
	}
}

func TestCompileDoctrineHighTech(t *testing.T) {
	d := Doctrine{
		Name:                  "Soviet Armor",
		EconomyPriority:       0.5,
		TechPriority:          0.7,
		GroundDefensePriority: 0.3,
		AirDefensePriority:    0.3,
		InfantryWeight:        0.3,
		VehicleWeight:         0.8,
		AirWeight:             0.3,
		NavalWeight:           0.3,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.5,
	}
	rules := CompileDoctrine(d)

	// Verify all rules compile with expr
	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	found := map[string]bool{}
	for _, r := range rules {
		found[r.Name] = true
	}

	// Tech center should be present
	if !found["build-tech-center"] {
		t.Error("expected build-tech-center with TechPriority=0.7")
	}

	// Heavy vehicle production should be present
	if !found["produce-heavy-vehicle"] {
		t.Error("expected produce-heavy-vehicle with TechPriority=0.7 and VehicleWeight=0.8")
	}

	// Rocket soldiers should be present
	if !found["produce-rocket-soldier"] {
		t.Error("expected produce-rocket-soldier with TechPriority=0.7 and InfantryWeight=0.3")
	}

	// Attack aircraft should be present (Air > 0.1 && Tech > 0.4)
	if !found["produce-attack-aircraft"] {
		t.Error("expected produce-attack-aircraft with TechPriority=0.7 and AirWeight=0.3")
	}

	// Advanced ship should be present (Naval > 0.1 && Tech > 0.3)
	if !found["produce-advanced-ship"] {
		t.Error("expected produce-advanced-ship with TechPriority=0.7 and NavalWeight=0.3")
	}

	// Extra war factory should be present (VehicleWeight > 0.6)
	if !found["build-extra-war-factory"] {
		t.Error("expected build-extra-war-factory with VehicleWeight=0.8")
	}

	// Advanced power should be present (Tech > 0.5)
	if !found["build-advanced-power"] {
		t.Error("expected build-advanced-power with TechPriority=0.7")
	}

	// Air attack rules should be present (AirWeight=0.3 > 0.1)
	if !found["form-air-attack"] {
		t.Error("expected form-air-attack with AirWeight=0.3")
	}
	if !found["squad-air-attack"] {
		t.Error("expected squad-air-attack with AirWeight=0.3")
	}

	// Naval attack rules should be present (NavalWeight=0.3 > 0.1)
	if !found["form-naval-attack"] {
		t.Error("expected form-naval-attack with NavalWeight=0.3")
	}
	if !found["squad-naval-attack"] {
		t.Error("expected squad-naval-attack with NavalWeight=0.3")
	}
}

func TestCompileDoctrineWithSuperweapons(t *testing.T) {
	d := Doctrine{
		Name:                  "Nuke Rush",
		EconomyPriority:       0.5,
		Aggression:            0.5,
		TechPriority:          0.7,
		InfantryWeight:        0.3,
		VehicleWeight:         0.5,
		AirWeight:             0.3,
		NavalWeight:           0.0,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.5,
		SuperweaponPriority:   0.7,
	}
	rules := CompileDoctrine(d)

	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	found := map[string]bool{}
	for _, r := range rules {
		found[r.Name] = true
	}

	// Building rules should be present (SuperweaponPriority > 0.3)
	if !found["build-missile-silo"] {
		t.Error("expected build-missile-silo with SuperweaponPriority=0.7")
	}
	if !found["build-iron-curtain"] {
		t.Error("expected build-iron-curtain with SuperweaponPriority=0.7")
	}

	// Fire rules should be present (SuperweaponPriority > 0.1)
	if !found["fire-nuke"] {
		t.Error("expected fire-nuke with SuperweaponPriority=0.7")
	}
	if !found["fire-iron-curtain"] {
		t.Error("expected fire-iron-curtain with SuperweaponPriority=0.7")
	}

	// Airfield power fire rules (AirWeight > 0.1)
	if !found["fire-spy-plane"] {
		t.Error("expected fire-spy-plane with AirWeight=0.3")
	}
	if !found["fire-paratroopers"] {
		t.Error("expected fire-paratroopers with AirWeight=0.3")
	}
	if !found["fire-parabombs"] {
		t.Error("expected fire-parabombs with AirWeight=0.3")
	}

	// Rebuild rules should always be present
	if !found["rebuild-missile-silo"] {
		t.Error("expected rebuild-missile-silo")
	}
	if !found["rebuild-iron-curtain"] {
		t.Error("expected rebuild-iron-curtain")
	}

	// All fire rules should be exclusive in "superweapon" category
	for _, r := range rules {
		if r.Name == "fire-nuke" || r.Name == "fire-iron-curtain" || r.Name == "fire-spy-plane" ||
			r.Name == "fire-paratroopers" || r.Name == "fire-parabombs" {
			if r.Category != "superweapon" {
				t.Errorf("rule %q should have category 'superweapon', got %q", r.Name, r.Category)
			}
			if !r.Exclusive {
				t.Errorf("rule %q should be exclusive", r.Name)
			}
		}
	}
}

func TestCompileDoctrineNoSuperweapons(t *testing.T) {
	d := Doctrine{
		Name:                  "No Superweapons",
		EconomyPriority:       0.5,
		Aggression:            0.5,
		InfantryWeight:        0.5,
		VehicleWeight:         0.5,
		AirWeight:             0.0,
		NavalWeight:           0.0,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		SuperweaponPriority:   0.0,
	}
	rules := CompileDoctrine(d)

	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	for _, r := range rules {
		switch r.Name {
		case "build-missile-silo", "build-iron-curtain",
			"fire-nuke", "fire-iron-curtain",
			"fire-spy-plane", "fire-spy-plane-update", "fire-paratroopers", "fire-parabombs":
			t.Errorf("unexpected superweapon rule %q when SuperweaponPriority=0 and AirWeight=0", r.Name)
		}
	}
}

func TestBuildCashCondition(t *testing.T) {
	tests := []struct {
		name     string
		unitCost int
		savings  []buildingSaving
		want     string
	}{
		{
			name:     "no savings",
			unitCost: 100,
			savings:  nil,
			want:     "Cash() >= 100",
		},
		{
			name:     "tech center only",
			unitCost: 100,
			savings:  []buildingSaving{{`HasRole("tech_center")`, 1500}},
			want:     `Cash() >= 100 && (HasRole("tech_center") || Cash() >= 1600)`,
		},
		{
			name:     "tech center and superweapon",
			unitCost: 100,
			savings: []buildingSaving{
				{`HasRole("tech_center")`, 1500},
				{`HasRole("missile_silo") || HasRole("iron_curtain")`, 2500},
			},
			want: `Cash() >= 100 && (HasRole("tech_center") || Cash() >= 1600) && (HasRole("missile_silo") || HasRole("iron_curtain") || Cash() >= 2600)`,
		},
		{
			name:     "expensive unit with savings",
			unitCost: 800,
			savings:  []buildingSaving{{`HasRole("tech_center")`, 1500}},
			want:     `Cash() >= 800 && (HasRole("tech_center") || Cash() >= 2300)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCashCondition(tt.unitCost, tt.savings)
			if got != tt.want {
				t.Errorf("buildCashCondition(%d, ...)\ngot:  %s\nwant: %s", tt.unitCost, got, tt.want)
			}
		})
	}
}

func TestCompileDoctrineBuildingSavings(t *testing.T) {
	// High tech + superweapon doctrine: production rules should have savings clauses.
	highTech := Doctrine{
		Name:                  "Technological Fortress",
		EconomyPriority:       0.5,
		Aggression:            0.5,
		TechPriority:          0.9,
		SuperweaponPriority:   0.85,
		InfantryWeight:        0.5,
		VehicleWeight:         0.5,
		AirWeight:             0.3,
		NavalWeight:           0.0,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.5,
	}
	highRules := CompileDoctrine(highTech)

	// All rules must compile.
	for _, r := range highRules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	// Production rules should contain savings clauses (HasRole checks).
	savingsRules := []string{
		"produce-infantry",
		"produce-vehicle",
		"produce-aircraft",
		"produce-rocket-soldier",
		"produce-heavy-vehicle",
		"produce-attack-aircraft",
	}
	for _, r := range highRules {
		for _, name := range savingsRules {
			if r.Name == name {
				if !strings.Contains(r.ConditionSrc, `HasRole("tech_center")`) {
					t.Errorf("rule %q should contain tech_center savings clause, got: %s", r.Name, r.ConditionSrc)
				}
				if !strings.Contains(r.ConditionSrc, `HasRole("missile_silo")`) {
					t.Errorf("rule %q should contain missile_silo savings clause, got: %s", r.Name, r.ConditionSrc)
				}
			}
		}
	}

	// Low tech doctrine: production rules should NOT have savings clauses.
	lowTech := Doctrine{
		Name:                  "Rush",
		EconomyPriority:       0.3,
		Aggression:            0.9,
		TechPriority:          0.1,
		SuperweaponPriority:   0.0,
		InfantryWeight:        0.8,
		VehicleWeight:         0.5,
		GroundAttackGroupSize: 4,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
	}
	lowRules := CompileDoctrine(lowTech)

	for _, r := range lowRules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	for _, r := range lowRules {
		if r.Name == "produce-infantry" || r.Name == "produce-vehicle" {
			if strings.Contains(r.ConditionSrc, `HasRole("tech_center")`) {
				t.Errorf("rule %q should NOT contain tech_center savings clause with TechPriority=0.1, got: %s", r.Name, r.ConditionSrc)
			}
		}
	}

	// Harvester rebuild should NOT have savings clauses (critical economy).
	for _, r := range highRules {
		if r.Name == "rebuild-harvester" {
			if strings.Contains(r.ConditionSrc, `HasRole("tech_center") || Cash()`) {
				t.Errorf("rebuild-harvester should NOT contain savings clauses, got: %s", r.ConditionSrc)
			}
		}
	}
}

