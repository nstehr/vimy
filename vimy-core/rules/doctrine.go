package rules

import "math"

// Doctrine represents a high-level strategic posture produced by the LLM.
// Weights are 0.0–1.0; the compiler maps them to concrete rule parameters.
type Doctrine struct {
	Name                  string  `json:"name"`
	Rationale             string  `json:"rationale"`
	EconomyPriority       float64 `json:"economy_priority"`
	Aggression            float64 `json:"aggression"`
	GroundDefensePriority float64 `json:"ground_defense_priority"`
	AirDefensePriority    float64 `json:"air_defense_priority"`
	TechPriority          float64 `json:"tech_priority"`
	InfantryWeight        float64 `json:"infantry_weight"`
	VehicleWeight         float64 `json:"vehicle_weight"`
	AirWeight             float64 `json:"air_weight"`
	NavalWeight           float64 `json:"naval_weight"`
	GroundAttackGroupSize int     `json:"ground_attack_group_size"`
	AirAttackGroupSize    int     `json:"air_attack_group_size"`
	NavalAttackGroupSize  int     `json:"naval_attack_group_size"`
	ScoutPriority              float64 `json:"scout_priority"`
	SpecializedInfantryWeight  float64 `json:"specialized_infantry_weight"`
}

// DefaultDoctrine returns a balanced baseline doctrine.
func DefaultDoctrine() Doctrine {
	return Doctrine{
		Name:                  "Balanced",
		Rationale:             "Default balanced strategy",
		EconomyPriority:       0.5,
		Aggression:            0.5,
		GroundDefensePriority: 0.5,
		AirDefensePriority:    0.3,
		TechPriority:          0.5,
		InfantryWeight:        0.5,
		VehicleWeight:         0.5,
		AirWeight:             0.0,
		NavalWeight:           0.0,
		GroundAttackGroupSize: 5,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
		ScoutPriority:         0.5,
	}
}

// Validate clamps all weights to their valid ranges.
func (d *Doctrine) Validate() {
	d.EconomyPriority = clamp(d.EconomyPriority, 0, 1)
	d.Aggression = clamp(d.Aggression, 0, 1)
	d.GroundDefensePriority = clamp(d.GroundDefensePriority, 0, 1)
	d.AirDefensePriority = clamp(d.AirDefensePriority, 0, 1)
	d.TechPriority = clamp(d.TechPriority, 0, 1)
	d.InfantryWeight = clamp(d.InfantryWeight, 0, 1)
	d.VehicleWeight = clamp(d.VehicleWeight, 0, 1)
	d.AirWeight = clamp(d.AirWeight, 0, 1)
	d.NavalWeight = clamp(d.NavalWeight, 0, 1)
	d.ScoutPriority = clamp(d.ScoutPriority, 0, 1)
	d.SpecializedInfantryWeight = clamp(d.SpecializedInfantryWeight, 0, 1)
	d.GroundAttackGroupSize = clampInt(d.GroundAttackGroupSize, 3, 15)
	d.AirAttackGroupSize = clampInt(d.AirAttackGroupSize, 1, 8)
	d.NavalAttackGroupSize = clampInt(d.NavalAttackGroupSize, 2, 10)
}

// clampInt restricts v to [min, max].
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// lerp linearly interpolates between min and max by t (0–1), returning an int.
func lerp(min, max int, t float64) int {
	return min + int(math.Round(float64(max-min)*t))
}

// lerpf linearly interpolates between min and max by t (0–1), returning a float64.
func lerpf(min, max, t float64) float64 {
	return min + (max-min)*t
}

// clamp restricts v to [min, max].
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
