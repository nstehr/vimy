package rules

import (
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
		"deploy-mcv":               false,
		"place-ready-building":     false,
		"place-ready-defense":      false,
		"cancel-stuck-aircraft":    false,
		"scramble-base-defense":    false,
		"scramble-naval-defense":   false,
		"repair-buildings":         false,
		"return-idle-harvesters":   false,
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
			"capture-building", "produce-engineer", "produce-apc",
			"load-engineer-into-apc", "deliver-apc-to-target":
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
	if ar.Category != "air_combat" {
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
	if nr.Category != "naval_combat" {
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

	// CapturePriority=0 → no capture rules
	t.Run("absent when CapturePriority=0", func(t *testing.T) {
		d := DefaultDoctrine()
		d.CapturePriority = 0
		rules := CompileDoctrine(d)

		found := map[string]bool{}
		for _, r := range rules {
			found[r.Name] = true
		}
		for _, name := range captureRuleNames {
			if found[name] {
				t.Errorf("unexpected capture rule %q when CapturePriority=0", name)
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
