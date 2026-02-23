package rules

import (
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
		"deploy-mcv":            false,
		"place-ready-building":  false,
		"place-ready-defense":   false,
		"repair-buildings":      false,
		"return-idle-harvesters": false,
		"capture-building":      false,
		"produce-engineer":      false,
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
			"produce-infantry", "produce-vehicle", "produce-aircraft", "produce-ship":
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
		"air-attack-enemy", "air-attack-known-base", "naval-attack-enemy",
		"build-base-defense", "build-aa-defense",
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
	if !found["air-attack-enemy"] {
		t.Error("expected air-attack-enemy with AirWeight=0.3")
	}

	// Naval attack rules should be present (NavalWeight=0.3 > 0.1)
	if !found["naval-attack-enemy"] {
		t.Error("expected naval-attack-enemy with NavalWeight=0.3")
	}
}
