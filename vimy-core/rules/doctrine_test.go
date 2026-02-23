package rules

import "testing"

func TestLerp(t *testing.T) {
	tests := []struct {
		min, max int
		t        float64
		want     int
	}{
		{5, 20, 0.0, 5},
		{5, 20, 1.0, 20},
		{5, 20, 0.5, 13}, // 5 + round(15*0.5) = 5 + 8 = 13
		{5, 20, 0.7, 16}, // 5 + round(15*0.7) = 5 + round(10.5) = 5 + 11 = 16
		{200, 400, 0.0, 200},
		{200, 400, 1.0, 400},
	}
	for _, tc := range tests {
		got := lerp(tc.min, tc.max, tc.t)
		if got != tc.want {
			t.Errorf("lerp(%d, %d, %.1f) = %d, want %d", tc.min, tc.max, tc.t, got, tc.want)
		}
	}
}

func TestLerpf(t *testing.T) {
	got := lerpf(0.0, 1.0, 0.5)
	if got != 0.5 {
		t.Errorf("lerpf(0, 1, 0.5) = %f, want 0.5", got)
	}
	got = lerpf(10.0, 20.0, 0.3)
	if got != 13.0 {
		t.Errorf("lerpf(10, 20, 0.3) = %f, want 13.0", got)
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, min, max, want float64
	}{
		{0.5, 0, 1, 0.5},
		{-0.5, 0, 1, 0.0},
		{1.5, 0, 1, 1.0},
		{0.0, 0, 1, 0.0},
		{1.0, 0, 1, 1.0},
	}
	for _, tc := range tests {
		got := clamp(tc.v, tc.min, tc.max)
		if got != tc.want {
			t.Errorf("clamp(%f, %f, %f) = %f, want %f", tc.v, tc.min, tc.max, got, tc.want)
		}
	}
}

func TestDefaultDoctrine(t *testing.T) {
	d := DefaultDoctrine()
	if d.Name != "Balanced" {
		t.Errorf("DefaultDoctrine().Name = %q, want %q", d.Name, "Balanced")
	}
	if d.EconomyPriority != 0.5 {
		t.Errorf("DefaultDoctrine().EconomyPriority = %f, want 0.5", d.EconomyPriority)
	}
	if d.GroundAttackGroupSize != 5 {
		t.Errorf("DefaultDoctrine().GroundAttackGroupSize = %d, want 5", d.GroundAttackGroupSize)
	}
	if d.AirAttackGroupSize != 2 {
		t.Errorf("DefaultDoctrine().AirAttackGroupSize = %d, want 2", d.AirAttackGroupSize)
	}
	if d.NavalAttackGroupSize != 3 {
		t.Errorf("DefaultDoctrine().NavalAttackGroupSize = %d, want 3", d.NavalAttackGroupSize)
	}
}

func TestValidate(t *testing.T) {
	d := Doctrine{
		EconomyPriority:       1.5,
		Aggression:            -0.5,
		InfantryWeight:        2.0,
		GroundAttackGroupSize: 1,
		AirAttackGroupSize:    0,
		NavalAttackGroupSize:  0,
	}
	d.Validate()

	if d.EconomyPriority != 1.0 {
		t.Errorf("EconomyPriority = %f, want 1.0 (clamped)", d.EconomyPriority)
	}
	if d.Aggression != 0.0 {
		t.Errorf("Aggression = %f, want 0.0 (clamped)", d.Aggression)
	}
	if d.InfantryWeight != 1.0 {
		t.Errorf("InfantryWeight = %f, want 1.0 (clamped)", d.InfantryWeight)
	}
	if d.GroundAttackGroupSize != 3 {
		t.Errorf("GroundAttackGroupSize = %d, want 3 (clamped)", d.GroundAttackGroupSize)
	}
	if d.AirAttackGroupSize != 1 {
		t.Errorf("AirAttackGroupSize = %d, want 1 (clamped)", d.AirAttackGroupSize)
	}
	if d.NavalAttackGroupSize != 2 {
		t.Errorf("NavalAttackGroupSize = %d, want 2 (clamped)", d.NavalAttackGroupSize)
	}

	if d.SpecializedInfantryWeight != 0.0 {
		t.Errorf("SpecializedInfantryWeight = %f, want 0.0 (clamped from unset)", d.SpecializedInfantryWeight)
	}

	d3 := Doctrine{SpecializedInfantryWeight: 1.5}
	d3.Validate()
	if d3.SpecializedInfantryWeight != 1.0 {
		t.Errorf("SpecializedInfantryWeight = %f, want 1.0 (clamped)", d3.SpecializedInfantryWeight)
	}

	d4 := Doctrine{SpecializedInfantryWeight: -0.5}
	d4.Validate()
	if d4.SpecializedInfantryWeight != 0.0 {
		t.Errorf("SpecializedInfantryWeight = %f, want 0.0 (clamped)", d4.SpecializedInfantryWeight)
	}

	d2 := Doctrine{GroundAttackGroupSize: 100, AirAttackGroupSize: 100, NavalAttackGroupSize: 100}
	d2.Validate()
	if d2.GroundAttackGroupSize != 15 {
		t.Errorf("GroundAttackGroupSize = %d, want 15 (clamped)", d2.GroundAttackGroupSize)
	}
	if d2.AirAttackGroupSize != 8 {
		t.Errorf("AirAttackGroupSize = %d, want 8 (clamped)", d2.AirAttackGroupSize)
	}
	if d2.NavalAttackGroupSize != 10 {
		t.Errorf("NavalAttackGroupSize = %d, want 10 (clamped)", d2.NavalAttackGroupSize)
	}
}
