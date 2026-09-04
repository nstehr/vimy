package rules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/nstehr/vimy/vimy-core/model"
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
		"deploy-mcv":             false,
		"place-ready-building":   false,
		"place-ready-defense":    false,
		"cancel-stuck-aircraft":  false,
		"scramble-base-defense":  false,
		"scramble-naval-defense": false,
		"repair-buildings":       false,
		"return-idle-harvesters": false,
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
			"fire-spy-plane", "fire-spy-plane-update", "fire-paratroopers", "fire-parabombs",
			"produce-engineer", "produce-apc":
			t.Errorf("unexpected military rule %q when all unit weights=0", r.Name)
		}
		// capture-building, load-engineer-into-apc, deliver-apc-to-target are
		// always emitted now (vimy-7e1) — their internal conditions gate
		// firing on CapturableCount/RoleCount, so emitting them when
		// CapturePriority=0 is harmless and prevents orphaning engineers
		// that survive a CapturePriority drop between doctrines.
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
		"produce-infantry", "produce-bridge-infantry", "produce-vehicle", "produce-aircraft", "produce-ship",
		"produce-specialist-infantry",
		"squad-reengage",
		"form-air-attack", "squad-air-attack", "squad-air-reengage", "squad-air-attack-known-base",
		"form-naval-attack", "squad-naval-attack", "squad-naval-reengage",
		"scramble-naval-defense",
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

func TestCompileDoctrineReengageRules(t *testing.T) {
	// Full-spectrum doctrine: all three re-engage rules should be emitted.
	d := Doctrine{
		Name:                  "Reengage Test",
		EconomyPriority:       0.5,
		Aggression:            0.7,
		GroundDefensePriority: 0.3,
		InfantryWeight:        0.5,
		VehicleWeight:         0.5,
		AirWeight:             0.5,
		NavalWeight:           0.5,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.5,
	}
	rules := CompileDoctrine(d)

	// All rules must compile.
	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	// Index rules by name for lookup.
	byName := map[string]*Rule{}
	for _, r := range rules {
		byName[r.Name] = r
	}

	// Ground re-engage
	gr := byName["squad-reengage"]
	if gr == nil {
		t.Fatal("expected squad-reengage rule")
	}
	attack := byName["squad-attack"]
	if attack == nil {
		t.Fatal("expected squad-attack rule")
	}
	if gr.Priority != attack.Priority-ReengageDiscount {
		t.Errorf("squad-reengage priority = %d, want %d (squad-attack %d - %d)",
			gr.Priority, attack.Priority-ReengageDiscount, attack.Priority, ReengageDiscount)
	}
	if gr.Category != "combat" {
		t.Errorf("squad-reengage category = %q, want \"combat\"", gr.Category)
	}
	if gr.Exclusive {
		t.Error("squad-reengage should be non-exclusive")
	}
	// Condition should NOT contain SquadReadyRatio (no ratio gate).
	if strings.Contains(gr.ConditionSrc, "SquadReadyRatio") {
		t.Errorf("squad-reengage should not gate on SquadReadyRatio, got: %s", gr.ConditionSrc)
	}

	// Air re-engage
	ar := byName["squad-air-reengage"]
	if ar == nil {
		t.Fatal("expected squad-air-reengage rule")
	}
	airAttack := byName["squad-air-attack"]
	if airAttack == nil {
		t.Fatal("expected squad-air-attack rule")
	}
	if ar.Priority != airAttack.Priority-ReengageDiscount {
		t.Errorf("squad-air-reengage priority = %d, want %d", ar.Priority, airAttack.Priority-ReengageDiscount)
	}
	if ar.Category != "air-combat" {
		t.Errorf("squad-air-reengage category = %q, want \"air_combat\"", ar.Category)
	}
	if strings.Contains(ar.ConditionSrc, "SquadReadyRatio") {
		t.Errorf("squad-air-reengage should not gate on SquadReadyRatio, got: %s", ar.ConditionSrc)
	}

	// Naval re-engage
	nr := byName["squad-naval-reengage"]
	if nr == nil {
		t.Fatal("expected squad-naval-reengage rule")
	}
	navalAttack := byName["squad-naval-attack"]
	if navalAttack == nil {
		t.Fatal("expected squad-naval-attack rule")
	}
	if nr.Priority != navalAttack.Priority-ReengageDiscount {
		t.Errorf("squad-naval-reengage priority = %d, want %d", nr.Priority, navalAttack.Priority-ReengageDiscount)
	}
	if nr.Category != "naval-combat" {
		t.Errorf("squad-naval-reengage category = %q, want \"naval_combat\"", nr.Category)
	}
	if strings.Contains(nr.ConditionSrc, "SquadReadyRatio") {
		t.Errorf("squad-naval-reengage should not gate on SquadReadyRatio, got: %s", nr.ConditionSrc)
	}
	if !strings.Contains(nr.ConditionSrc, "MapHasWater()") {
		t.Errorf("squad-naval-reengage should require MapHasWater(), got: %s", nr.ConditionSrc)
	}

	// Doctrine with no air/naval: re-engage rules for those domains should be absent.
	groundOnly := Doctrine{
		Name:                  "Ground Only",
		Aggression:            0.7,
		InfantryWeight:        0.5,
		VehicleWeight:         0.5,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
	}
	groundRules := CompileDoctrine(groundOnly)
	for _, r := range groundRules {
		if r.Name == "squad-air-reengage" {
			t.Error("unexpected squad-air-reengage when AirWeight=0")
		}
		if r.Name == "squad-naval-reengage" {
			t.Error("unexpected squad-naval-reengage when NavalWeight=0")
		}
	}
	foundGround := false
	for _, r := range groundRules {
		if r.Name == "squad-reengage" {
			foundGround = true
		}
	}
	if !foundGround {
		t.Error("expected squad-reengage even when air/naval weights are 0")
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

func TestCompileDoctrineHighNaval(t *testing.T) {
	d := Doctrine{
		Name:                  "Naval Dominance",
		EconomyPriority:       0.5,
		Aggression:            0.5,
		TechPriority:          0.5,
		GroundDefensePriority: 0.3,
		InfantryWeight:        0.3,
		VehicleWeight:         0.3,
		AirWeight:             0.0,
		NavalWeight:           0.8,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  4,
		ScoutPriority:         0.5,
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

	if !found["build-naval-yard"] {
		t.Error("expected build-naval-yard with NavalWeight=0.8")
	}
	if !found["build-extra-naval-yard"] {
		t.Error("expected build-extra-naval-yard with NavalWeight=0.8 (> DoctrineExtreme)")
	}
	if !found["scramble-naval-defense"] {
		t.Error("expected scramble-naval-defense as core rule")
	}
	if !found["produce-ship"] {
		t.Error("expected produce-ship with NavalWeight=0.8")
	}
	if !found["produce-advanced-ship"] {
		t.Error("expected produce-advanced-ship with NavalWeight=0.8 and TechPriority=0.5")
	}
	if !found["form-naval-attack"] {
		t.Error("expected form-naval-attack with NavalWeight=0.8")
	}
	if !found["squad-naval-attack"] {
		t.Error("expected squad-naval-attack with NavalWeight=0.8")
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

func TestQueueBusyMultipleQueues(t *testing.T) {
	tests := []struct {
		name   string
		queues []model.ProductionQueue
		want   bool
	}{
		{
			name:   "no matching queues",
			queues: []model.ProductionQueue{{Type: "Vehicle"}},
			want:   false,
		},
		{
			name: "single queue busy",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 50},
			},
			want: true,
		},
		{
			name: "single queue idle",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "", CurrentProgress: 0},
			},
			want: false,
		},
		{
			name: "single queue complete",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 100},
			},
			want: false,
		},
		{
			name: "two queues both busy",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 50},
				{Type: "Ship", CurrentItem: "dd", CurrentProgress: 30},
			},
			want: true,
		},
		{
			name: "two queues first busy second idle",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 50},
				{Type: "Ship", CurrentItem: "", CurrentProgress: 0},
			},
			want: false,
		},
		{
			name: "two queues first idle second busy",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "", CurrentProgress: 0},
				{Type: "Ship", CurrentItem: "dd", CurrentProgress: 30},
			},
			want: false,
		},
		{
			name: "two queues first busy second complete",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 50},
				{Type: "Ship", CurrentItem: "dd", CurrentProgress: 100},
			},
			want: false,
		},
		{
			name: "mixed queue types only ship checked",
			queues: []model.ProductionQueue{
				{Type: "Vehicle", CurrentItem: "tank", CurrentProgress: 50},
				{Type: "Ship", CurrentItem: "", CurrentProgress: 0},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := RuleEnv{State: model.GameState{ProductionQueues: tt.queues}}
			got := env.QueueBusy("Ship")
			if got != tt.want {
				t.Errorf("QueueBusy(\"Ship\") = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQueueReadyMultipleQueues(t *testing.T) {
	tests := []struct {
		name   string
		queues []model.ProductionQueue
		want   bool
	}{
		{
			name:   "no matching queues",
			queues: []model.ProductionQueue{{Type: "Vehicle"}},
			want:   false,
		},
		{
			name: "single queue not ready",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 50},
			},
			want: false,
		},
		{
			name: "single queue ready",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 100},
			},
			want: true,
		},
		{
			name: "two queues first not ready second ready",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 50},
				{Type: "Ship", CurrentItem: "dd", CurrentProgress: 100},
			},
			want: true,
		},
		{
			name: "two queues first ready second not ready",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 100},
				{Type: "Ship", CurrentItem: "dd", CurrentProgress: 30},
			},
			want: true,
		},
		{
			name: "two queues neither ready",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "ss", CurrentProgress: 50},
				{Type: "Ship", CurrentItem: "dd", CurrentProgress: 30},
			},
			want: false,
		},
		{
			name: "two queues both idle",
			queues: []model.ProductionQueue{
				{Type: "Ship", CurrentItem: "", CurrentProgress: 0},
				{Type: "Ship", CurrentItem: "", CurrentProgress: 0},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := RuleEnv{State: model.GameState{ProductionQueues: tt.queues}}
			got := env.QueueReady("Ship")
			if got != tt.want {
				t.Errorf("QueueReady(\"Ship\") = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompileDoctrineCaptureRules(t *testing.T) {
	captureRuleNames := []string{
		"capture-building",
		"produce-engineer",
		"produce-apc",
		"load-engineer-into-apc",
		"deliver-apc-to-target",
	}

	// CapturePriority=0 → only PRODUCTION rules absent. Direction rules
	// (capture-building, load-engineer-into-apc, deliver-apc-to-target) are
	// always emitted to handle orphaned engineers/APCs that survive a
	// capture_priority drop between doctrines (vimy-7e1).
	productionOnly := []string{"produce-engineer", "produce-apc"}
	directionOnly := []string{"capture-building", "load-engineer-into-apc", "deliver-apc-to-target"}
	t.Run("production absent, direction present when CapturePriority=0", func(t *testing.T) {
		d := DefaultDoctrine()
		d.CapturePriority = 0
		rules := CompileDoctrine(d)

		found := map[string]bool{}
		for _, r := range rules {
			found[r.Name] = true
		}
		for _, name := range productionOnly {
			if found[name] {
				t.Errorf("unexpected production capture rule %q when CapturePriority=0", name)
			}
		}
		for _, name := range directionOnly {
			if !found[name] {
				t.Errorf("expected direction capture rule %q to be present when CapturePriority=0 (always emitted to handle orphans)", name)
			}
		}
	})

	// CapturePriority=0.5 → all capture rules present
	t.Run("present when CapturePriority=0.5", func(t *testing.T) {
		d := DefaultDoctrine()
		d.CapturePriority = 0.5
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
		for _, name := range captureRuleNames {
			if !found[name] {
				t.Errorf("expected capture rule %q when CapturePriority=0.5", name)
			}
		}
	})

	// Engineer cap scales with CapturePriority
	t.Run("engineer cap scales with priority", func(t *testing.T) {
		// Low priority (0.2) → engineer cap = lerp(1,3,0.2) = 1
		low := DefaultDoctrine()
		low.CapturePriority = 0.2
		lowRules := CompileDoctrine(low)
		for _, r := range lowRules {
			if r.Name == "produce-engineer" {
				if !strings.Contains(r.ConditionSrc, `RoleCount("engineer") < 1`) {
					t.Errorf("low CapturePriority: expected engineer cap 1, got condition: %s", r.ConditionSrc)
				}
			}
		}

		// High priority (1.0) → engineer cap = lerp(1,3,1.0) = 3
		high := DefaultDoctrine()
		high.CapturePriority = 1.0
		highRules := CompileDoctrine(high)
		for _, r := range highRules {
			if r.Name == "produce-engineer" {
				if !strings.Contains(r.ConditionSrc, `RoleCount("engineer") < 3`) {
					t.Errorf("high CapturePriority: expected engineer cap 3, got condition: %s", r.ConditionSrc)
				}
			}
		}

		// Mid priority (0.5) → engineer cap = lerp(1,3,0.5) = 2
		mid := DefaultDoctrine()
		mid.CapturePriority = 0.5
		midRules := CompileDoctrine(mid)
		for _, r := range midRules {
			if r.Name == "produce-engineer" {
				if !strings.Contains(r.ConditionSrc, `RoleCount("engineer") < 2`) {
					t.Errorf("mid CapturePriority: expected engineer cap 2, got condition: %s", r.ConditionSrc)
				}
			}
		}
	})

	// Conservative capture (below DoctrineSignificant): keep the
	// CapturableCount() > 0 gates so low-priority doctrines don't waste queues.
	t.Run("conservative capture keeps visibility gates", func(t *testing.T) {
		d := DefaultDoctrine()
		d.CapturePriority = 0.2 // below DoctrineSignificant (0.3)
		byName := map[string]*Rule{}
		for _, r := range CompileDoctrine(d) {
			byName[r.Name] = r
		}
		for _, name := range []string{"produce-engineer", "produce-apc", "deliver-apc-to-target"} {
			r := byName[name]
			if r == nil {
				t.Fatalf("expected %q rule", name)
			}
			if !strings.Contains(r.ConditionSrc, `CapturableCount() > 0`) {
				t.Errorf("%s: expected CapturableCount() > 0 gate, got: %s", name, r.ConditionSrc)
			}
		}
	})

	// Aggressive capture (>= DoctrineSignificant): drop the visibility gate so
	// engineers/APCs build proactively and loaded APCs double as scouts.
	t.Run("aggressive capture drops visibility gates", func(t *testing.T) {
		d := DefaultDoctrine()
		d.CapturePriority = 0.5
		byName := map[string]*Rule{}
		for _, r := range CompileDoctrine(d) {
			byName[r.Name] = r
		}
		for _, name := range []string{"produce-engineer", "produce-apc", "deliver-apc-to-target"} {
			r := byName[name]
			if r == nil {
				t.Fatalf("expected %q rule", name)
			}
			if strings.Contains(r.ConditionSrc, `CapturableCount() > 0`) {
				t.Errorf("%s: expected no CapturableCount() > 0 gate at aggressive priority, got: %s", name, r.ConditionSrc)
			}
		}
	})

	// load-engineer-into-apc must exclude engineers within capture range, else
	// a just-unloaded engineer gets re-loaded before capture-building fires.
	t.Run("load-engineer guards against re-load race", func(t *testing.T) {
		d := DefaultDoctrine()
		d.CapturePriority = 0.5
		for _, r := range CompileDoctrine(d) {
			if r.Name == "load-engineer-into-apc" {
				if !strings.Contains(r.ConditionSrc, `!EngineerNearCapturable()`) {
					t.Errorf("load-engineer-into-apc missing !EngineerNearCapturable() guard: %s", r.ConditionSrc)
				}
				return
			}
		}
		t.Fatal("load-engineer-into-apc rule not present")
	})
}

func TestBuildRadarPriorityRelativeToMilitary(t *testing.T) {
	// Vehicle doctrine: war factory must beat radar in the "economy"
	// exclusive category so APC production isn't delayed by radar.
	t.Run("vehicle doctrine: war factory beats radar", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.5
		d.Aggression = 0.5
		d.CapturePriority = 0.5
		byName := compileToMap(d)
		radar := byName["build-radar"]
		wf := byName["build-war-factory"]
		if radar == nil || wf == nil {
			t.Fatalf("expected both rules; got radar=%v wf=%v", radar, wf)
		}
		if wf.Priority <= radar.Priority {
			t.Errorf("war-factory priority (%d) must be > radar priority (%d)", wf.Priority, radar.Priority)
		}
	})

	t.Run("air doctrine: airfield beats radar", func(t *testing.T) {
		d := DefaultDoctrine()
		d.AirWeight = 0.5
		byName := compileToMap(d)
		radar := byName["build-radar"]
		af := byName["build-airfield"]
		if radar == nil || af == nil {
			t.Fatalf("expected both rules; got radar=%v airfield=%v", radar, af)
		}
		if af.Priority <= radar.Priority {
			t.Errorf("airfield priority (%d) must be > radar priority (%d)", af.Priority, radar.Priority)
		}
	})

	// Pure-tech doctrine: no military building rules. Radar should still
	// compile, barracks-prereq should still beat radar so firing order
	// (barracks → radar) matches today's behavior.
	t.Run("pure-tech doctrine: barracks-prereq beats radar, radar present", func(t *testing.T) {
		d := DefaultDoctrine()
		d.InfantryWeight = 0
		d.VehicleWeight = 0
		d.AirWeight = 0
		d.NavalWeight = 0
		d.TechPriority = 0.5
		d.GroundDefensePriority = 0
		byName := compileToMap(d)
		radar := byName["build-radar"]
		prereq := byName["build-barracks-prereq"]
		if radar == nil {
			t.Fatal("pure-tech doctrine must still compile build-radar")
		}
		if prereq == nil {
			t.Fatal("pure-tech doctrine must compile build-barracks-prereq")
		}
		if prereq.Priority <= radar.Priority {
			t.Errorf("barracks-prereq priority (%d) must be > radar priority (%d) so barracks still fires first", prereq.Priority, radar.Priority)
		}
	})
}

// build-radar priority must bump when the doctrine's primary preferred
// vehicle requires radar (V2, heavy tank, etc.) but must stay low when the
// primary pref is APC or another non-radar-gated unit. Without the narrow
// gate on PreferredVehicle[0], APC-rush doctrines that list medium_tank
// as a secondary pref would get radar built before the war factory and
// regress to the original pre-drop behavior.
func TestBuildRadarPriorityScalesWithPrimaryVehiclePref(t *testing.T) {
	t.Run("V2 primary: radar bumps to 710", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.5
		d.PreferredVehicle = []string{"v2_launcher", "heavy_tank", "medium_tank"}
		byName := compileToMap(d)
		radar := byName["build-radar"]
		if radar == nil {
			t.Fatal("build-radar rule missing")
		}
		if radar.Priority != 710 {
			t.Errorf("expected radar priority 710 for V2-primary doctrine, got %d", radar.Priority)
		}
	})

	t.Run("APC primary with tank secondaries: radar stays at 570", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.4
		d.CapturePriority = 0.6
		d.TransportAssault = 0.8
		d.PreferredVehicle = []string{"apc", "medium_tank", "heavy_tank", "tesla_tank"}
		byName := compileToMap(d)
		radar := byName["build-radar"]
		if radar == nil {
			t.Fatal("build-radar rule missing")
		}
		if radar.Priority != 570 {
			t.Errorf("expected radar priority 570 for APC-primary doctrine (prevents APC-rush regression), got %d", radar.Priority)
		}
	})

	t.Run("V2 primary: radar slots between second-refinery and war factory", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.5
		d.EconomyPriority = 0.7
		d.PreferredVehicle = []string{"v2_launcher"}
		byName := compileToMap(d)
		radar := byName["build-radar"]
		wf := byName["build-war-factory"]
		secondRef := byName["build-second-refinery"]
		if radar.Priority <= secondRef.Priority {
			t.Errorf("V2 doctrine: radar (%d) must outrank second-refinery (%d)", radar.Priority, secondRef.Priority)
		}
		if radar.Priority >= wf.Priority {
			t.Errorf("V2 doctrine: radar (%d) must stay below war-factory (%d) so WF builds first", radar.Priority, wf.Priority)
		}
	})
}

// produce-vehicle's cash gate must scale with VehicleWeight so the tech
// center reserve doesn't fully lock out tank production when the doctrine
// explicitly prioritized vehicles. Observed live (VehicleWeight=0.55,
// TechPriority=0.6): 0 produce-vehicle fires over a 17k-tick game because
// the combined reserves pushed the effective threshold above 3000 cash.
func TestProduceVehicleCashGate_ScalesWithVehicleWeight(t *testing.T) {
	t.Run("high vehicle + tech: cash gate reduced from tech reserve", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.55
		d.TechPriority = 0.6
		d.EconomyPriority = 0.7
		byName := compileToMap(d)
		veh := byName["produce-vehicle"]
		if veh == nil {
			t.Fatal("produce-vehicle rule missing")
		}
		// At VehicleWeight=0.55 the scale is (1-0.55)=0.45. Tech-center
		// reserve of 1500 scales to ~675 instead of 1500. Effective high
		// threshold ≈ 800+675=1475 instead of 800+1500=2300.
		if !strings.Contains(veh.ConditionSrc, "Cash() >= 800") {
			t.Errorf("expected base cash gate in condition: %s", veh.ConditionSrc)
		}
		if strings.Contains(veh.ConditionSrc, "Cash() >= 2300") {
			t.Errorf("expected scaled tech-center reserve, still see full 1500 reserve: %s", veh.ConditionSrc)
		}
	})

	t.Run("low vehicle: full reserves preserved", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.2 // below DoctrineHigh — no scaling
		d.TechPriority = 0.5  // TechPriority > DoctrineHigh so tech reserve is active
		byName := compileToMap(d)
		veh := byName["produce-vehicle"]
		if veh == nil {
			t.Fatal("produce-vehicle rule missing")
		}
		// Should preserve full 1500 tech reserve → Cash() >= 2300 clause present.
		if !strings.Contains(veh.ConditionSrc, "Cash() >= 2300") {
			t.Errorf("expected full tech reserve (2300 clause) for low-vehicle doctrine, got: %s", veh.ConditionSrc)
		}
	})
}

// produce-extra-harvester must outrank combat-vehicle rules in the exclusive
// CatProduceVehicle queue — otherwise aggressive doctrines (high
// CapturePriority / TransportAssault) starve income expansion behind APCs and
// flak trucks and the base runs on its single starter harvester forever.
// Cash gate must also be bare (no c.savings stack) since harvesters generate
// the income those reserves exist to protect.
// produce-attack-dog must outrank rifles/specialists/rocket-soldiers/engineers
// in the exclusive CatProduceInfantry queue. Otherwise the first dog never
// gets built, no scout gets designated, and scout-with-scouts never fires —
// leaving Soviet-faction doctrines with no non-APC scouting at all.
func TestAttackDogPriorityBeatsRifles(t *testing.T) {
	d := DefaultDoctrine()
	d.InfantryWeight = 0.6
	d.CapturePriority = 0.6
	d.ScoutPriority = 0.9
	d.TechPriority = 0.4 // enough for rocket-soldier to compile

	byName := compileToMap(d)
	dog := byName["produce-attack-dog"]
	rifle := byName["produce-infantry"]
	if dog == nil {
		t.Fatal("produce-attack-dog rule missing with InfantryWeight=0.6")
	}
	if rifle == nil {
		t.Fatal("produce-infantry rule missing with InfantryWeight=0.6")
	}
	if dog.Priority <= rifle.Priority {
		t.Errorf("produce-attack-dog (%d) must outrank produce-infantry (%d) so first dog actually gets built", dog.Priority, rifle.Priority)
	}
	// Also above any other infantry production we commonly compile.
	for _, name := range []string{"produce-rocket-soldier", "produce-specialist-infantry", "produce-engineer"} {
		r := byName[name]
		if r == nil {
			continue
		}
		if dog.Priority <= r.Priority {
			t.Errorf("produce-attack-dog (%d) must outrank %s (%d)", dog.Priority, name, r.Priority)
		}
	}
}

// When the doctrine prefers v2_launcher / artillery, build-war-factory must
// outrank build-second-refinery in the exclusive "economy" queue. Otherwise
// the second refinery wins and cheaper buildings snipe the economy queue
// until cash for the war factory is never met, delaying the siege pipeline
// by 5+ minutes. Observed live: V2-doctrine got war factory at tick 7550
// of a 13k-tick game, zero siege vehicles produced.
func TestWarFactoryPriorityWhenSiegePreferred(t *testing.T) {
	d := DefaultDoctrine()
	d.VehicleWeight = 0.6
	d.EconomyPriority = 0.75 // pushes second-refinery priority to 665
	d.PreferredVehicle = []string{"v2_launcher", "heavy_tank"}
	byName := compileToMap(d)
	wf := byName["build-war-factory"]
	secondRef := byName["build-second-refinery"]
	if wf == nil || secondRef == nil {
		t.Fatalf("expected both rules; wf=%v secondRef=%v", wf, secondRef)
	}
	if wf.Priority <= secondRef.Priority {
		t.Errorf("war-factory (%d) must outrank second-refinery (%d) when siege is preferred", wf.Priority, secondRef.Priority)
	}
}

// When the doctrine explicitly prefers v2_launcher or artillery,
// produce-siege-vehicle must outrank produce-vehicle and produce-flak-truck
// so the preferred siege unit actually gets built. Without this bump, a
// 53-minute stand-off-bombardment doctrine produced 1 siege vehicle.
func TestSiegeVehiclePriorityWhenPreferred(t *testing.T) {
	t.Run("preferred siege bumps priority above vehicle+flak", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.6
		d.TechPriority = 0.5
		d.AirDefensePriority = 0.5 // enables flak-truck rule
		d.PreferredVehicle = []string{"v2_launcher", "heavy_tank"}
		byName := compileToMap(d)
		siege := byName["produce-siege-vehicle"]
		veh := byName["produce-vehicle"]
		flak := byName["produce-flak-truck"]
		if siege == nil || veh == nil {
			t.Fatalf("expected siege+vehicle rules; siege=%v vehicle=%v", siege, veh)
		}
		if siege.Priority <= veh.Priority {
			t.Errorf("preferred siege (%d) must outrank produce-vehicle (%d)", siege.Priority, veh.Priority)
		}
		if flak != nil && siege.Priority <= flak.Priority {
			t.Errorf("preferred siege (%d) must outrank produce-flak-truck (%d)", siege.Priority, flak.Priority)
		}
	})

	t.Run("preferred siege: cash gate scales with VehicleWeight", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.6
		d.TechPriority = 0.5 // tech-center reserve active
		d.PreferredVehicle = []string{"v2_launcher"}
		byName := compileToMap(d)
		siege := byName["produce-siege-vehicle"]
		if siege == nil {
			t.Fatal("produce-siege-vehicle missing")
		}
		// At VW=0.6, scale = 0.4, tech reserve 1500 → 600; siege gate
		// should NOT show a full 2400 clause (base 900 + full 1500).
		if strings.Contains(siege.ConditionSrc, "Cash() >= 2400") {
			t.Errorf("expected siege cash gate scaled, still see full 2400: %s", siege.ConditionSrc)
		}
		if !strings.Contains(siege.ConditionSrc, "Cash() >= 900") {
			t.Errorf("expected base 900 threshold in siege condition: %s", siege.ConditionSrc)
		}
	})

	t.Run("not preferred: siege stays default (below produce-vehicle)", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.6
		d.TechPriority = 0.5
		d.PreferredVehicle = []string{"heavy_tank"} // no siege pref
		byName := compileToMap(d)
		siege := byName["produce-siege-vehicle"]
		veh := byName["produce-vehicle"]
		if siege == nil || veh == nil {
			t.Fatalf("expected siege+vehicle rules; siege=%v vehicle=%v", siege, veh)
		}
		if siege.Priority >= veh.Priority {
			t.Errorf("non-preferred siege (%d) should stay below produce-vehicle (%d) so tanks dominate", siege.Priority, veh.Priority)
		}
	})
}

func TestExtraHarvesterPriorityAndCashGate(t *testing.T) {
	d := DefaultDoctrine()
	d.EconomyPriority = 0.7    // > DoctrineDominant so rule compiles
	d.CapturePriority = 0.8    // produce-apc + savings active
	d.TransportAssault = 0.8   // produce-assault-apc active
	d.VehicleWeight = 0.45     // produce-vehicle active
	d.AirDefensePriority = 0.5 // produce-flak-truck active

	byName := compileToMap(d)
	harv := byName["produce-extra-harvester"]
	if harv == nil {
		t.Fatalf("produce-extra-harvester rule missing with EconomyPriority=0.7")
	}
	// Must be strictly above every competing vehicle-queue rule.
	competitors := []string{"produce-vehicle", "produce-assault-apc", "produce-apc", "produce-flak-truck"}
	for _, name := range competitors {
		r := byName[name]
		if r == nil {
			continue
		}
		if harv.Priority <= r.Priority {
			t.Errorf("produce-extra-harvester (%d) must outrank %s (%d) so income expansion isn't starved", harv.Priority, name, r.Priority)
		}
	}
	// Cash gate must be bare Cash() >= 1400 (no reserve stacking). Harvesters
	// generate the income those reserves protect — gating income on them is
	// backwards. Accept either "Cash() >= 1400" or "Cash() >= 1400 " (trailing
	// whitespace variants) but fail if an arithmetic operator follows 1400.
	if !strings.Contains(harv.ConditionSrc, "Cash() >= 1400") {
		t.Fatalf("expected 'Cash() >= 1400' in condition, got: %s", harv.ConditionSrc)
	}
	idx := strings.Index(harv.ConditionSrc, "Cash() >= 1400")
	tail := strings.TrimSpace(harv.ConditionSrc[idx+len("Cash() >= 1400"):])
	if strings.HasPrefix(tail, "+") {
		t.Errorf("cash gate is stacking a reserve: %s", harv.ConditionSrc)
	}
}

func TestInfantrySavingsRushAware(t *testing.T) {
	// The infantry-specific war-factory reserve ("infantry can't spend if
	// war factory isn't built yet, period") is the unconditional clause
	// HasRole("war_factory") || Cash() >= infantryCost+2000. Aggressive
	// doctrines drop it; conservative doctrines keep it.
	//
	// Counting occurrences of `HasRole("war_factory")` in the condition
	// distinguishes the two cases: c.savings contributes one clause with
	// `HasRole("war_factory") || !HasRole("radar")` (radar-conditional,
	// present in both). The infantry-specific reserve adds a second,
	// unconditional `HasRole("war_factory")` — the one that hurts rush.
	t.Run("aggressive doctrine: only the radar-conditional war-factory clause", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.3
		d.Aggression = 0.5 // >= DoctrineSignificant
		d.InfantryWeight = 0.5
		byName := compileToMap(d)
		inf := byName["produce-infantry"]
		if inf == nil {
			t.Fatal("expected produce-infantry rule")
		}
		n := strings.Count(inf.ConditionSrc, `HasRole("war_factory")`)
		if n != 1 {
			t.Errorf("aggressive: expected 1 war-factory clause (radar-conditional only), got %d in: %s", n, inf.ConditionSrc)
		}
		// Verify the one that remains IS the radar-conditional one.
		if !strings.Contains(inf.ConditionSrc, `HasRole("war_factory") || !HasRole("radar")`) {
			t.Errorf("aggressive: expected radar-conditional clause preserved, got: %s", inf.ConditionSrc)
		}
	})

	t.Run("conservative doctrine: both clauses present", func(t *testing.T) {
		d := DefaultDoctrine()
		d.VehicleWeight = 0.3
		d.Aggression = 0.2 // < DoctrineSignificant
		d.InfantryWeight = 0.5
		byName := compileToMap(d)
		inf := byName["produce-infantry"]
		if inf == nil {
			t.Fatal("expected produce-infantry rule")
		}
		n := strings.Count(inf.ConditionSrc, `HasRole("war_factory")`)
		if n != 2 {
			t.Errorf("conservative: expected 2 war-factory clauses (radar-conditional + infantry-specific), got %d in: %s", n, inf.ConditionSrc)
		}
	})
}

func TestCaptureDefenseInfantryFloor(t *testing.T) {
	// Pure engineer-rush doctrine: InfantryWeight = 0 means produce-infantry
	// is NOT compiled, so engineers are the only infantry. Engineers can't
	// shoot. The defense floor rule gives the base 3 rifles to defend with
	// regardless of InfantryWeight.
	t.Run("pure engineer-rush compiles defense floor", func(t *testing.T) {
		d := DefaultDoctrine()
		d.InfantryWeight = 0
		d.CapturePriority = 0.5
		byName := compileToMap(d)
		if _, ok := byName["produce-infantry"]; ok {
			t.Fatal("precondition broken: produce-infantry should not compile at InfantryWeight=0")
		}
		floor := byName["produce-capture-defense-infantry"]
		if floor == nil {
			t.Fatal("expected produce-capture-defense-infantry for rush doctrine")
		}
		eng := byName["produce-engineer"]
		if eng == nil {
			t.Fatal("expected produce-engineer for capture doctrine")
		}
		if floor.Priority >= eng.Priority {
			t.Errorf("defense floor priority (%d) must be below produce-engineer (%d) so engineers win when they can build", floor.Priority, eng.Priority)
		}
	})

	// Non-capture doctrines must NOT get the defense floor — it's specifically
	// a rush-safety net.
	t.Run("absent when CapturePriority is 0", func(t *testing.T) {
		d := DefaultDoctrine()
		d.CapturePriority = 0
		byName := compileToMap(d)
		if _, ok := byName["produce-capture-defense-infantry"]; ok {
			t.Error("defense floor should not compile for non-capture doctrines")
		}
	})

	// Infantry-heavy doctrine: produce-infantry (500) supersedes the floor (440).
	// The floor still compiles but never fires in practice.
	t.Run("infantry-heavy: produce-infantry outranks defense floor", func(t *testing.T) {
		d := DefaultDoctrine()
		d.InfantryWeight = 0.7
		d.CapturePriority = 0.5
		byName := compileToMap(d)
		floor := byName["produce-capture-defense-infantry"]
		inf := byName["produce-infantry"]
		if floor == nil || inf == nil {
			t.Fatalf("expected both rules; floor=%v inf=%v", floor, inf)
		}
		if inf.Priority <= floor.Priority {
			t.Errorf("produce-infantry priority (%d) must be > defense-floor priority (%d) so it takes the queue when compiled", inf.Priority, floor.Priority)
		}
	})
}

func TestDeliverAssaultAPCIntelGate(t *testing.T) {
	// Aggressive TransportAssault drops the HasEnemyIntel() gate so
	// combat-loaded APCs can explore when no building has been sighted.
	t.Run("aggressive: no intel gate", func(t *testing.T) {
		d := DefaultDoctrine()
		d.TransportAssault = 0.5
		r := compileToMap(d)["deliver-assault-apc"]
		if r == nil {
			t.Fatal("expected deliver-assault-apc")
		}
		if strings.Contains(r.ConditionSrc, "HasEnemyIntel()") {
			t.Errorf("aggressive TransportAssault: intel gate should be dropped, got: %s", r.ConditionSrc)
		}
	})

	// Low TransportAssault keeps the gate so combat APCs don't wander
	// randomly when the doctrine isn't committed to an assault plan.
	t.Run("low: intel gate preserved", func(t *testing.T) {
		d := DefaultDoctrine()
		d.TransportAssault = 0.15
		r := compileToMap(d)["deliver-assault-apc"]
		if r == nil {
			t.Fatal("expected deliver-assault-apc")
		}
		if !strings.Contains(r.ConditionSrc, "HasEnemyIntel()") {
			t.Errorf("low TransportAssault: intel gate should be preserved, got: %s", r.ConditionSrc)
		}
	})
}

func compileToMap(d Doctrine) map[string]*Rule {
	out := map[string]*Rule{}
	for _, r := range CompileDoctrine(d) {
		out[r.Name] = r
	}
	return out
}

func TestCompileDoctrineEngineerPriority(t *testing.T) {
	// Infantry swarm with capture: produce-engineer should have lower priority
	// than produce-infantry so engineers don't steal the shared Infantry queue.
	d := Doctrine{
		Name:                  "Infantry Swarm",
		EconomyPriority:       0.5,
		Aggression:            0.7,
		InfantryWeight:        0.9,
		VehicleWeight:         0.3,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		CapturePriority:       0.3,
	}
	rules := CompileDoctrine(d)

	byName := map[string]*Rule{}
	for _, r := range rules {
		byName[r.Name] = r
	}

	eng := byName["produce-engineer"]
	inf := byName["produce-infantry"]
	if eng == nil {
		t.Fatal("expected produce-engineer rule with CapturePriority=0.3")
	}
	if inf == nil {
		t.Fatal("expected produce-infantry rule with InfantryWeight=0.9")
	}
	if eng.Priority >= inf.Priority {
		t.Errorf("produce-engineer priority (%d) should be below produce-infantry priority (%d)", eng.Priority, inf.Priority)
	}

	// Also verify produce-apc is below produce-vehicle
	apc := byName["produce-apc"]
	veh := byName["produce-vehicle"]
	if apc == nil {
		t.Fatal("expected produce-apc rule with CapturePriority=0.3")
	}
	if veh == nil {
		t.Fatal("expected produce-vehicle rule with VehicleWeight=0.3")
	}
	if apc.Priority >= veh.Priority {
		t.Errorf("produce-apc priority (%d) should be below produce-vehicle priority (%d)", apc.Priority, veh.Priority)
	}
}

func TestBestAirTarget(t *testing.T) {
	base := model.Building{X: 0, Y: 0}

	t.Run("prefers defense over unit at same distance", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "3tnk", X: 10, Y: 0, HP: 100, MaxHP: 100},
				{ID: 2, Type: "tsla", X: 10, Y: 0, HP: 200, MaxHP: 200},
			},
		}}
		got := env.BestAirTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected tsla (ID=2), got %+v", got)
		}
	})

	t.Run("damaged target gets bonus over full-HP same type", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "tsla", X: 10, Y: 0, HP: 200, MaxHP: 200},
				{ID: 2, Type: "tsla", X: 10, Y: 0, HP: 50, MaxHP: 200},
			},
		}}
		got := env.BestAirTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected damaged tsla (ID=2), got %+v", got)
		}
	})

	t.Run("nearby lower-value beats distant higher-value at extreme range", func(t *testing.T) {
		// gun at dist=5: score = 8 * 1.0 / sqrt(5) ≈ 3.58
		// tsla at dist=10000: score = 10 * 1.0 / sqrt(10000) = 0.10
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "gun", X: 5, Y: 0, HP: 100, MaxHP: 100},
				{ID: 2, Type: "tsla", X: 10000, Y: 0, HP: 200, MaxHP: 200},
			},
		}}
		got := env.BestAirTarget()
		if got == nil || got.ID != 1 {
			t.Errorf("expected nearby gun (ID=1), got %+v", got)
		}
	})

	t.Run("faction variant stripping", func(t *testing.T) {
		// afld.ukraine should be scored as afld (value=5), not default (1)
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "e1", X: 10, Y: 0, HP: 50, MaxHP: 50},
				{ID: 2, Type: "afld.ukraine", X: 10, Y: 0, HP: 100, MaxHP: 100},
			},
		}}
		got := env.BestAirTarget()
		if got == nil || got.ID != 2 {
			t.Errorf("expected afld.ukraine (ID=2), got %+v", got)
		}
	})

	t.Run("returns nil when empty", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies:   nil,
		}}
		if got := env.BestAirTarget(); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("skips MaxHP=0 enemies", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Enemies: []model.Enemy{
				{ID: 1, Type: "tsla", X: 10, Y: 0, HP: 0, MaxHP: 0},
			},
		}}
		if got := env.BestAirTarget(); got != nil {
			t.Errorf("expected nil for MaxHP=0 enemy, got %+v", got)
		}
	})
}

func TestCompileDoctrineAirStrikeRules(t *testing.T) {
	d := Doctrine{
		Name:                  "Air Strike Test",
		EconomyPriority:       0.3,
		Aggression:            0.5,
		InfantryWeight:        0.3,
		VehicleWeight:         0.3,
		AirWeight:             0.5,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    3,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.3,
	}
	rules := CompileDoctrine(d)

	// All rules must compile.
	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	byName := map[string]*Rule{}
	for _, r := range rules {
		byName[r.Name] = r
	}

	// squad-air-attack should use BestAirTarget, not NearestEnemy
	airAttack := byName["squad-air-attack"]
	if airAttack == nil {
		t.Fatal("expected squad-air-attack rule")
	}
	if !strings.Contains(airAttack.ConditionSrc, "BestAirTarget()") {
		t.Errorf("squad-air-attack condition should contain BestAirTarget(), got: %s", airAttack.ConditionSrc)
	}
	if strings.Contains(airAttack.ConditionSrc, "NearestEnemy()") {
		t.Errorf("squad-air-attack condition should NOT contain NearestEnemy(), got: %s", airAttack.ConditionSrc)
	}

	// squad-air-reengage should use BestAirTarget, not NearestEnemy
	airReengage := byName["squad-air-reengage"]
	if airReengage == nil {
		t.Fatal("expected squad-air-reengage rule")
	}
	if !strings.Contains(airReengage.ConditionSrc, "BestAirTarget()") {
		t.Errorf("squad-air-reengage condition should contain BestAirTarget(), got: %s", airReengage.ConditionSrc)
	}
	if strings.Contains(airReengage.ConditionSrc, "NearestEnemy()") {
		t.Errorf("squad-air-reengage condition should NOT contain NearestEnemy(), got: %s", airReengage.ConditionSrc)
	}
}

func findRule(rules []*Rule, name string) *Rule {
	for _, r := range rules {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func TestCompileDoctrine_MicroRulesPresent(t *testing.T) {
	d := DefaultDoctrine()
	d.Aggression = 0.3
	rules := CompileDoctrine(d)

	expected := []string{
		"retreat-damaged-units",
		"clear-healed-units",
		"recall-overextended-ground-attack",
		"recall-overextended-naval-attack",
		"squad-disengage-ground-attack",
		"squad-disengage-naval-attack",
	}
	for _, name := range expected {
		if findRule(rules, name) == nil {
			t.Errorf("expected rule %q to be present with aggression=0.3", name)
		}
	}
}

func TestCompileDoctrine_RetreatUsesDamagedCombatUnits(t *testing.T) {
	d := DefaultDoctrine()
	rules := CompileDoctrine(d)

	r := findRule(rules, "retreat-damaged-units")
	if r == nil {
		t.Fatal("retreat-damaged-units rule not found")
	}
	if !strings.Contains(r.ConditionSrc, "DamagedCombatUnits") {
		t.Errorf("retreat rule should use DamagedCombatUnits, got: %s", r.ConditionSrc)
	}
	if strings.Contains(r.ConditionSrc, "DamagedSquadUnits") {
		t.Errorf("retreat rule should NOT use DamagedSquadUnits, got: %s", r.ConditionSrc)
	}
}

func TestCompileDoctrine_ClearHealedAlwaysOn(t *testing.T) {
	d := DefaultDoctrine()
	rules := CompileDoctrine(d)

	r := findRule(rules, "clear-healed-units")
	if r == nil {
		t.Fatal("clear-healed-units rule not found")
	}
	if r.ConditionSrc != "HasRetreatingUnits()" {
		t.Errorf("clear-healed-units should have condition 'HasRetreatingUnits()', got: %s", r.ConditionSrc)
	}
	if r.Priority != 500 {
		t.Errorf("clear-healed-units priority should be 500, got: %d", r.Priority)
	}
	if r.Exclusive {
		t.Error("clear-healed-units should not be exclusive")
	}
}

func TestCompileDoctrine_DisengageAbsentAtFullAggression(t *testing.T) {
	d := DefaultDoctrine()
	d.Aggression = 1.0
	rules := CompileDoctrine(d)

	if findRule(rules, "squad-disengage-ground-attack") != nil {
		t.Error("squad-disengage-ground-attack should NOT be present at aggression=1.0")
	}
	if findRule(rules, "squad-disengage-naval-attack") != nil {
		t.Error("squad-disengage-naval-attack should NOT be present at aggression=1.0")
	}
}

func TestCompileDoctrine_RecallAlwaysPresent(t *testing.T) {
	d := DefaultDoctrine()
	d.Aggression = 1.0
	rules := CompileDoctrine(d)

	if findRule(rules, "recall-overextended-ground-attack") == nil {
		t.Error("recall-overextended-ground-attack should be present at any aggression")
	}
}

func TestCompileDoctrine_MicroPriorities(t *testing.T) {
	d := DefaultDoctrine()
	d.Aggression = 0.5
	rules := CompileDoctrine(d)

	retreat := findRule(rules, "retreat-damaged-units")
	recall := findRule(rules, "recall-overextended-ground-attack")
	disengage := findRule(rules, "squad-disengage-ground-attack")
	clearHealed := findRule(rules, "clear-healed-units")

	if retreat == nil || recall == nil || disengage == nil || clearHealed == nil {
		t.Fatal("expected all micro rules present")
	}

	// clear-healed > retreat > disengage > recall
	if clearHealed.Priority <= retreat.Priority {
		t.Errorf("clear-healed priority (%d) should be > retreat priority (%d)", clearHealed.Priority, retreat.Priority)
	}
	if retreat.Priority <= disengage.Priority {
		t.Errorf("retreat priority (%d) should be > disengage priority (%d)", retreat.Priority, disengage.Priority)
	}
	if disengage.Priority <= recall.Priority {
		t.Errorf("disengage priority (%d) should be > recall priority (%d)", disengage.Priority, recall.Priority)
	}
}

func TestBestCapturable(t *testing.T) {
	base := model.Building{X: 0, Y: 0}

	t.Run("prefers oil derrick over hospital at same distance", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Capturables: []model.Enemy{
				{ID: 1, Type: "hosp", X: 10, Y: 0},
				{ID: 2, Type: "oilb", X: 10, Y: 0},
			},
		}}
		got := env.BestCapturable()
		if got == nil || got.ID != 2 {
			t.Errorf("expected oilb (ID=2), got %+v", got)
		}
	})

	t.Run("nearby hospital beats distant oil derrick", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Capturables: []model.Enemy{
				{ID: 1, Type: "hosp", X: 3, Y: 0},
				{ID: 2, Type: "oilb", X: 100, Y: 0},
			},
		}}
		got := env.BestCapturable()
		if got == nil || got.ID != 1 {
			t.Errorf("expected nearby hosp (ID=1), got %+v", got)
		}
	})

	t.Run("oil derrick wins when moderately closer", func(t *testing.T) {
		// oilb at dist=20, hosp at dist=10
		// oilb score: 10/sqrt(20)=2.24, hosp score: 3/sqrt(10)=0.95
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Capturables: []model.Enemy{
				{ID: 1, Type: "hosp", X: 10, Y: 0},
				{ID: 2, Type: "oilb", X: 20, Y: 0},
			},
		}}
		got := env.BestCapturable()
		if got == nil || got.ID != 2 {
			t.Errorf("expected oilb (ID=2), got %+v", got)
		}
	})

	t.Run("unknown type gets default value", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings: []model.Building{base},
			Capturables: []model.Enemy{
				{ID: 1, Type: "v19", X: 10, Y: 0},
			},
		}}
		got := env.BestCapturable()
		if got == nil || got.ID != 1 {
			t.Errorf("expected unknown capturable (ID=1), got %+v", got)
		}
	})

	t.Run("empty returns nil", func(t *testing.T) {
		env := RuleEnv{State: model.GameState{
			Buildings:   []model.Building{base},
			Capturables: nil,
		}}
		if got := env.BestCapturable(); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})
}

// ruleFingerprint returns a string that captures a compiled doctrine's
// behavioral identity: which rules exist and their priorities/conditions.
func ruleFingerprint(rules []*Rule) string {
	var parts []string
	for _, r := range rules {
		parts = append(parts, fmt.Sprintf("%s|%d|%s", r.Name, r.Priority, r.ConditionSrc))
	}
	return strings.Join(parts, "\n")
}

// ruleNames returns the sorted set of rule names for a compiled doctrine.
func ruleNames(rules []*Rule) map[string]bool {
	m := make(map[string]bool, len(rules))
	for _, r := range rules {
		m[r.Name] = true
	}
	return m
}

// TestDoctrineConvergence compiles several representative doctrines and
// verifies that they produce distinct rule sets — different rule names,
// different priorities, or different condition parameters.
func TestDoctrineConvergence(t *testing.T) {
	base := func() Doctrine {
		d := DefaultDoctrine()
		d.Validate()
		return d
	}

	doctrines := map[string]Doctrine{
		"balanced": base(),

		"infantry-rush": {
			Name: "Infantry Rush", EconomyPriority: 0.3, Aggression: 0.9,
			InfantryWeight: 0.8, VehicleWeight: 0.15, TechPriority: 0.1,
			GroundDefensePriority: 0.1, AirDefensePriority: 0.0,
			ScoutPriority: 0.7, GroundAttackGroupSize: 4, AirAttackGroupSize: 2, NavalAttackGroupSize: 3,
		},

		"armour-blitz": {
			Name: "Armour Blitz", EconomyPriority: 0.45, Aggression: 0.8,
			InfantryWeight: 0.2, VehicleWeight: 0.8, TechPriority: 0.5,
			GroundDefensePriority: 0.2, AirDefensePriority: 0.2,
			ScoutPriority: 0.5, GroundAttackGroupSize: 6, AirAttackGroupSize: 2, NavalAttackGroupSize: 3,
		},

		"turtle-tech": {
			Name: "Turtle Tech", EconomyPriority: 0.8, Aggression: 0.2,
			InfantryWeight: 0.4, VehicleWeight: 0.5, TechPriority: 0.8,
			GroundDefensePriority: 0.8, AirDefensePriority: 0.7,
			ScoutPriority: 0.3, SuperweaponPriority: 0.6,
			GroundAttackGroupSize: 10, AirAttackGroupSize: 2, NavalAttackGroupSize: 3,
		},

		"air-superiority": {
			Name: "Air Superiority", EconomyPriority: 0.5, Aggression: 0.6,
			InfantryWeight: 0.2, VehicleWeight: 0.3, AirWeight: 0.7, TechPriority: 0.6,
			GroundDefensePriority: 0.3, AirDefensePriority: 0.5,
			ScoutPriority: 0.5, GroundAttackGroupSize: 5, AirAttackGroupSize: 4, NavalAttackGroupSize: 3,
		},

		"naval-assault": {
			Name: "Naval Assault", EconomyPriority: 0.6, Aggression: 0.5,
			InfantryWeight: 0.2, VehicleWeight: 0.2, NavalWeight: 0.7, TechPriority: 0.4,
			GroundDefensePriority: 0.4, AirDefensePriority: 0.3,
			ScoutPriority: 0.5, GroundAttackGroupSize: 5, AirAttackGroupSize: 2, NavalAttackGroupSize: 5,
		},

		"engineer-rush": {
			Name: "Engineer Rush", EconomyPriority: 0.3, Aggression: 0.7,
			InfantryWeight: 0.0, VehicleWeight: 0.3, TechPriority: 0.1,
			CapturePriority: 0.8, TransportAssault: 0.0,
			GroundDefensePriority: 0.1, AirDefensePriority: 0.0,
			ScoutPriority: 0.8, GroundAttackGroupSize: 3, AirAttackGroupSize: 2, NavalAttackGroupSize: 3,
		},

		"specialist-force": {
			Name: "Specialist Force", EconomyPriority: 0.5, Aggression: 0.6,
			InfantryWeight: 0.5, VehicleWeight: 0.5, TechPriority: 0.5,
			SpecializedInfantryWeight: 0.7,
			GroundDefensePriority:     0.3, AirDefensePriority: 0.3,
			ScoutPriority: 0.5, GroundAttackGroupSize: 6, AirAttackGroupSize: 2, NavalAttackGroupSize: 3,
			PreferredInfantry: []string{"flamethrower", "shock_trooper"},
		},

		"apc-assault": {
			Name: "APC Assault", EconomyPriority: 0.4, Aggression: 0.8,
			InfantryWeight: 0.3, VehicleWeight: 0.5, TechPriority: 0.2,
			TransportAssault: 0.7, CapturePriority: 0.2,
			GroundDefensePriority: 0.2, AirDefensePriority: 0.1,
			ScoutPriority: 0.5, GroundAttackGroupSize: 5, AirAttackGroupSize: 2, NavalAttackGroupSize: 3,
		},
	}

	// Validate all doctrines.
	for name := range doctrines {
		d := doctrines[name]
		d.Validate()
		doctrines[name] = d
	}

	// Compile all doctrines.
	compiled := make(map[string][]*Rule)
	fingerprints := make(map[string]string)
	for name, d := range doctrines {
		rules := CompileDoctrine(d)
		compiled[name] = rules
		fingerprints[name] = ruleFingerprint(rules)
	}

	// --- Check 1: No two doctrines produce identical fingerprints ---
	for nameA, fpA := range fingerprints {
		for nameB, fpB := range fingerprints {
			if nameA >= nameB {
				continue
			}
			if fpA == fpB {
				t.Errorf("CONVERGED: %q and %q produce identical rule sets", nameA, nameB)
			}
		}
	}

	// --- Check 2: Rule count diversity ---
	counts := make(map[string]int)
	for name, rules := range compiled {
		counts[name] = len(rules)
	}
	minCount, maxCount := 9999, 0
	for _, c := range counts {
		if c < minCount {
			minCount = c
		}
		if c > maxCount {
			maxCount = c
		}
	}
	t.Logf("rule count range: %d – %d", minCount, maxCount)
	if maxCount-minCount < 5 {
		t.Errorf("rule count spread too narrow (%d – %d); doctrines may be converging", minCount, maxCount)
	}

	// --- Check 3: Specific structural differences ---

	// Infantry rush should NOT have war factory or tech center.
	infantryRush := compiled["infantry-rush"]
	if findRule(infantryRush, "build-war-factory") != nil {
		t.Log("NOTE: infantry-rush includes war factory (vehicle_weight=0.15 > DoctrineEnabled)")
	}
	if findRule(infantryRush, "build-tech-center") != nil {
		t.Error("infantry-rush should NOT have build-tech-center (tech=0.1)")
	}

	// Turtle-tech should have superweapon buildings and tech center.
	turtleTech := compiled["turtle-tech"]
	if findRule(turtleTech, "build-tech-center") == nil {
		t.Error("turtle-tech missing build-tech-center")
	}
	if findRule(turtleTech, "build-missile-silo") == nil {
		t.Error("turtle-tech missing build-missile-silo")
	}

	// Air superiority should have airfield but naval-assault should not.
	airSup := compiled["air-superiority"]
	navalAss := compiled["naval-assault"]
	if findRule(airSup, "build-airfield") == nil {
		t.Error("air-superiority missing build-airfield")
	}
	if findRule(navalAss, "build-airfield") != nil {
		t.Error("naval-assault should NOT have build-airfield (air=0)")
	}
	if findRule(navalAss, "build-naval-yard") == nil {
		t.Error("naval-assault missing build-naval-yard")
	}

	// Engineer rush should have capture rules but no infantry production.
	engRush := compiled["engineer-rush"]
	if findRule(engRush, "capture-building") == nil {
		t.Error("engineer-rush missing capture-building")
	}
	if findRule(engRush, "produce-infantry") != nil {
		t.Error("engineer-rush should NOT produce infantry (infantry_weight=0)")
	}

	// --- Check 4: Build order priorities differ ---
	// War factory priority should differ between armour-blitz (vehicle=0.8)
	// and balanced (vehicle=0.5).
	blitzWF := findRule(compiled["armour-blitz"], "build-war-factory")
	balancedWF := findRule(compiled["balanced"], "build-war-factory")
	if blitzWF != nil && balancedWF != nil {
		if blitzWF.Priority == balancedWF.Priority {
			t.Errorf("armour-blitz and balanced should have different war factory priorities (both %d)", blitzWF.Priority)
		}
		if blitzWF.Priority <= balancedWF.Priority {
			t.Errorf("armour-blitz war factory priority (%d) should be > balanced (%d)", blitzWF.Priority, balancedWF.Priority)
		}
		t.Logf("war factory priorities: armour-blitz=%d, balanced=%d", blitzWF.Priority, balancedWF.Priority)
	}

	// Extra refinery priority should differ between turtle-tech (economy=0.8)
	// and infantry-rush (economy=0.3).
	turtleRef := findRule(turtleTech, "build-extra-refinery")
	rushRef := findRule(infantryRush, "build-extra-refinery")
	if turtleRef != nil && rushRef != nil {
		if turtleRef.Priority <= rushRef.Priority {
			t.Errorf("turtle-tech extra-refinery priority (%d) should be > infantry-rush (%d)", turtleRef.Priority, rushRef.Priority)
		}
		t.Logf("extra-refinery priorities: turtle-tech=%d, infantry-rush=%d", turtleRef.Priority, rushRef.Priority)
	}

	// --- Check 5: Attack group sizes propagate ---
	// Verify condition strings contain the doctrine's group size.
	blitzAttack := findRule(compiled["armour-blitz"], "form-ground-attack-squad")
	turtleAttack := findRule(compiled["turtle-tech"], "form-ground-attack-squad")
	if blitzAttack != nil && turtleAttack != nil {
		if !strings.Contains(blitzAttack.ConditionSrc, "6") {
			t.Errorf("armour-blitz attack squad condition should reference group size 6: %s", blitzAttack.ConditionSrc)
		}
		if !strings.Contains(turtleAttack.ConditionSrc, "10") {
			t.Errorf("turtle-tech attack squad condition should reference group size 10: %s", turtleAttack.ConditionSrc)
		}
	}

	// --- Print summary ---
	for name, rules := range compiled {
		names := ruleNames(rules)
		var exclusive []string
		for _, r := range rules {
			if r.Exclusive {
				exclusive = append(exclusive, r.Name)
			}
		}
		t.Logf("%-20s rules=%d exclusive=%d", name, len(names), len(exclusive))
	}
}

func TestCompileDoctrineBridgeInfantry(t *testing.T) {
	// Mixed doctrine with air and naval: bridge infantry should be present.
	mixed := Doctrine{
		Name:                  "Air-Sea Mixed",
		EconomyPriority:       0.5,
		Aggression:            0.6,
		GroundDefensePriority: 0.3,
		InfantryWeight:        0.45,
		VehicleWeight:         0.15,
		AirWeight:             0.6,
		NavalWeight:           0.3,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    3,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.5,
	}
	rules := CompileDoctrine(mixed)

	// All rules must compile.
	for _, r := range rules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}

	byName := map[string]*Rule{}
	for _, r := range rules {
		byName[r.Name] = r
	}

	br := byName["produce-bridge-infantry"]
	if br == nil {
		t.Fatal("expected produce-bridge-infantry rule for mixed doctrine")
	}

	// Condition should check for missing buildings.
	if !strings.Contains(br.ConditionSrc, `!HasRole("airfield")`) {
		t.Errorf("bridge infantry should check for missing airfield, got: %s", br.ConditionSrc)
	}
	if !strings.Contains(br.ConditionSrc, `!HasRole("naval_yard")`) {
		t.Errorf("bridge infantry should check for missing naval_yard, got: %s", br.ConditionSrc)
	}

	// Should be in the infantry production category and exclusive.
	if br.Category != CatProduceInfantry {
		t.Errorf("bridge infantry category = %q, want %q", br.Category, CatProduceInfantry)
	}
	if !br.Exclusive {
		t.Error("bridge infantry should be exclusive")
	}

	// Priority should be lower than normal infantry production.
	inf := byName["produce-infantry"]
	if inf == nil {
		t.Fatal("expected produce-infantry rule")
	}
	if br.Priority >= inf.Priority {
		t.Errorf("bridge infantry priority %d should be less than infantry priority %d", br.Priority, inf.Priority)
	}

	// Pure infantry doctrine: bridge infantry should NOT be present.
	pureInf := Doctrine{
		Name:                  "Pure Infantry",
		EconomyPriority:       0.5,
		Aggression:            0.7,
		GroundDefensePriority: 0.5,
		InfantryWeight:        0.9,
		VehicleWeight:         0.05,
		AirWeight:             0.05,
		NavalWeight:           0.0,
		GroundAttackGroupSize: 8,
		ScoutPriority:         0.5,
	}
	pureRules := CompileDoctrine(pureInf)
	for _, r := range pureRules {
		if r.Name == "produce-bridge-infantry" {
			t.Error("pure infantry doctrine should NOT have produce-bridge-infantry rule")
		}
	}

	// Vehicle-heavy doctrine (heavy tank): bridge infantry should check for
	// missing war_factory.
	heavyTank := Doctrine{
		Name:                  "Heavy Tank",
		EconomyPriority:       0.5,
		Aggression:            0.8,
		GroundDefensePriority: 0.3,
		InfantryWeight:        0.3,
		VehicleWeight:         0.8,
		AirWeight:             0.1,
		NavalWeight:           0.0,
		GroundAttackGroupSize: 5,
		ScoutPriority:         0.5,
	}
	tankRules := CompileDoctrine(heavyTank)
	for _, r := range tankRules {
		_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
		}
	}
	tankByName := map[string]*Rule{}
	for _, r := range tankRules {
		tankByName[r.Name] = r
	}
	tankBridge := tankByName["produce-bridge-infantry"]
	if tankBridge == nil {
		t.Fatal("expected produce-bridge-infantry rule for heavy tank doctrine")
	}
	if !strings.Contains(tankBridge.ConditionSrc, `!HasRole("war_factory")`) {
		t.Errorf("heavy tank bridge infantry should check for missing war_factory, got: %s", tankBridge.ConditionSrc)
	}
}

func TestCompileDoctrine_CommitRatio(t *testing.T) {
	// commit_ratio > 0 overrides the aggression-derived activation threshold.
	d := DefaultDoctrine()
	d.Aggression = 0.5
	d.CommitRatio = 0.3
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	sa := byName["squad-attack"]
	if sa == nil {
		t.Fatal("squad-attack rule missing")
	}
	if !strings.Contains(sa.ConditionSrc, `>= 0.30`) {
		t.Errorf("expected commit_ratio 0.30 in squad-attack condition, got: %s", sa.ConditionSrc)
	}
}

func TestCompileDoctrine_CommitRatioZeroUsesDefault(t *testing.T) {
	d := DefaultDoctrine()
	d.Aggression = 0.5
	// CommitRatio zero → default lerp(0.6, 1.0, 1 - 0.5) = 0.8
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	sa := byName["squad-attack"]
	if sa == nil {
		t.Fatal("squad-attack rule missing")
	}
	if strings.Contains(sa.ConditionSrc, `>= 0.30`) {
		t.Errorf("commit_ratio should NOT be 0.30 when unset, got: %s", sa.ConditionSrc)
	}
}

func TestCompileDoctrine_BaseDefenseFloorGates(t *testing.T) {
	d := DefaultDoctrine()
	d.BaseDefenseFloor = 4
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	sa := byName["squad-attack"]
	sakb := byName["squad-attack-known-base"]
	if sa == nil || sakb == nil {
		t.Fatal("squad-attack or squad-attack-known-base missing")
	}
	if !strings.Contains(sa.ConditionSrc, `>= 4`) {
		t.Errorf("squad-attack should gate on defense floor 4, got: %s", sa.ConditionSrc)
	}
	if !strings.Contains(sakb.ConditionSrc, `>= 4`) {
		t.Errorf("squad-attack-known-base should gate on defense floor 4, got: %s", sakb.ConditionSrc)
	}
}

func TestCompileDoctrine_BaseDefenseFloorZeroDoesNotGate(t *testing.T) {
	d := DefaultDoctrine()
	d.BaseDefenseFloor = 0
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	sa := byName["squad-attack"]
	if sa == nil {
		t.Fatal("squad-attack missing")
	}
	if strings.Contains(sa.ConditionSrc, `RoleCount("pillbox")`) {
		t.Errorf("squad-attack should not gate on defense floor when 0, got: %s", sa.ConditionSrc)
	}
}

func TestCompileDoctrine_ExtraAirfieldFiresAtAirEnabled(t *testing.T) {
	// Was gated on DoctrineExtreme (0.4). Lowered to DoctrineEnabled (0.1)
	// so modest air-doctrines can still build enough pads.
	d := DefaultDoctrine()
	d.AirWeight = 0.20
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	if byName["build-extra-airfield"] == nil {
		t.Fatal("build-extra-airfield should compile when AirWeight > DoctrineEnabled")
	}
}

func TestCompileDoctrine_ExtraAirfieldGatesOnPhysicalCapacity(t *testing.T) {
	d := DefaultDoctrine()
	d.AirWeight = 0.7
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	r := byName["build-extra-airfield"]
	if r == nil {
		t.Fatal("build-extra-airfield missing")
	}
	if !strings.Contains(r.ConditionSrc, "AircraftCapacity() <") {
		t.Errorf("extra-airfield should gate on doctrinal cap via AircraftCapacity, got: %s", r.ConditionSrc)
	}
	if !strings.Contains(r.ConditionSrc, "CombatAircraftCount() >= AircraftCapacity() - 1") {
		t.Errorf("extra-airfield should gate on near-full pads, got: %s", r.ConditionSrc)
	}
	if strings.Contains(r.ConditionSrc, `RoleCount("airfield") <`) {
		t.Errorf("extra-airfield should no longer use RoleCount cap, got: %s", r.ConditionSrc)
	}
}

func TestCompileDoctrine_BarracksBeatsRadarForRadarGatedPref(t *testing.T) {
	// Game 63 pattern: preferred_vehicle=[medium_tank ...] pushed radar
	// priority to 710, which beat un-floored barracks (670 at these
	// weights). Barracks priority floor of 745 should now win.
	d := DefaultDoctrine()
	d.InfantryWeight = 0.3
	d.VehicleWeight = 0.6
	d.GroundDefensePriority = 0.7
	d.EconomyPriority = 0.7
	d.TechPriority = 0.5
	d.PreferredVehicle = []string{"medium_tank", "heavy_tank"}
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	barr := byName["build-barracks"]
	radar := byName["build-radar"]
	if barr == nil || radar == nil {
		t.Fatal("expected both build-barracks and build-radar")
	}
	if barr.Priority <= radar.Priority {
		t.Errorf("build-barracks priority (%d) must beat build-radar (%d) for combined-arms doctrine with radar-gated preferred vehicle", barr.Priority, radar.Priority)
	}
}

func TestCompileDoctrine_InfantrySavingsScalesWithInfantryWeight(t *testing.T) {
	// Combined-arms: infantry=0.28, vehicle=0.62. Reservation scale =
	// (0.62 - 0.28) = 0.34, so reserve = int(800 * 0.34) = 272. Infantry
	// needs cash >= 100 + 272 = 372, not >= 900 (old bug).
	d := DefaultDoctrine()
	d.InfantryWeight = 0.28
	d.VehicleWeight = 0.62
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	pi := byName["produce-infantry"]
	if pi == nil {
		t.Fatal("produce-infantry missing")
	}
	// Old bug: condition contained "Cash() >= 900". New: should be lower.
	if strings.Contains(pi.ConditionSrc, "Cash() >= 900") {
		t.Errorf("produce-infantry still uses full 800-reserve for combined-arms doctrine, got: %s", pi.ConditionSrc)
	}
}

func TestCompileDoctrine_InfantryHeavyDoctrineHasNoVehicleReserve(t *testing.T) {
	// Infantry-heavy: infantry=0.7, vehicle=0.4. Reservation scale =
	// max(0, 0.4-0.7) = 0. Zero reservation should be present.
	d := DefaultDoctrine()
	d.InfantryWeight = 0.7
	d.VehicleWeight = 0.4
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	pi := byName["produce-infantry"]
	if pi == nil {
		t.Fatal("produce-infantry missing")
	}
	// With reserve=0, the CombatVehicleCount clause should not be added.
	if strings.Contains(pi.ConditionSrc, "CombatVehicleCount()") {
		t.Errorf("infantry-heavy doctrine should NOT reserve for vehicles, got: %s", pi.ConditionSrc)
	}
}

func TestCompileDoctrine_PureVehicleDoctrineKeepsFullReserve(t *testing.T) {
	// Pure vehicle (infantry=0.1 doesn't compile infantry rule at all, so
	// use 0.15 to keep the rule alive). vehicle=0.8. Reserve scale =
	// (0.8 - 0.15) = 0.65, reserve = int(800 * 0.65) = 520.
	d := DefaultDoctrine()
	d.InfantryWeight = 0.15
	d.VehicleWeight = 0.8
	compiled := CompileDoctrine(d)
	byName := map[string]*Rule{}
	for _, r := range compiled {
		byName[r.Name] = r
	}
	pi := byName["produce-infantry"]
	if pi == nil {
		t.Fatal("produce-infantry missing")
	}
	// Should have a meaningful vehicle reserve clause.
	if !strings.Contains(pi.ConditionSrc, "CombatVehicleCount()") {
		t.Errorf("vehicle-heavy doctrine should reserve for vehicles, got: %s", pi.ConditionSrc)
	}
}
