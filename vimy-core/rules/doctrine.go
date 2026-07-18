package rules

import "math"

// Doctrine is the LLM's output — a strategic posture expressed as continuous
// 0–1 weights. CompileDoctrine translates these into discrete rule sets.
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
	SuperweaponPriority        float64 `json:"superweapon_priority"`
	CapturePriority            float64  `json:"capture_priority"`
	PreferredInfantry          []string `json:"preferred_infantry,omitempty"`
	PreferredVehicle           []string `json:"preferred_vehicle,omitempty"`
	PreferredAircraft          []string `json:"preferred_aircraft,omitempty"`
	PreferredNaval             []string `json:"preferred_naval,omitempty"`
	TransportAssault           float64  `json:"transport_assault,omitempty"`
	GroundTargetAAPriority     float64  `json:"ground_target_aa_priority"`
	AirTargetGroundDefPriority float64  `json:"air_target_ground_def_priority"`

	// Tempo / commit knobs (added in the 4-knob expansion). Zero-value
	// defaults preserve prior behavior when unset.
	CommitRatio        float64 `json:"commit_ratio"`         // 0.0-1.0, 0=disabled → use lerp default. Ready-ratio threshold for squad deployment.
	BaseDefenseFloor   int     `json:"base_defense_floor"`   // 0-15, minimum base defenses before offensive squad rules fire.
	RepairBudgetRatio  float64 `json:"repair_budget_ratio"`  // 0.0-1.0, 0=disabled → unlimited (current behavior). Cap on repair spending as a fraction of cash on hand.
	ScoutReachPriority float64 `json:"scout_reach_priority"` // 0.0-1.0, 0=disabled → default perimeter patrol. Higher = probe toward last-known enemy direction.
}

// DefaultDoctrine is used when no LLM strategist is configured.
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

// Validate sanitizes LLM output — the model may produce out-of-range values.
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
	d.SuperweaponPriority = clamp(d.SuperweaponPriority, 0, 1)
	d.CapturePriority = clamp(d.CapturePriority, 0, 1)
	d.TransportAssault = clamp(d.TransportAssault, 0, 1)
	d.GroundTargetAAPriority = clamp(d.GroundTargetAAPriority, 0, 1)
	d.AirTargetGroundDefPriority = clamp(d.AirTargetGroundDefPriority, 0, 1)
	d.GroundAttackGroupSize = clampInt(d.GroundAttackGroupSize, 3, 15)
	d.AirAttackGroupSize = clampInt(d.AirAttackGroupSize, 1, 8)
	d.NavalAttackGroupSize = clampInt(d.NavalAttackGroupSize, 2, 10)
	d.CommitRatio = clamp(d.CommitRatio, 0, 1)
	d.BaseDefenseFloor = clampInt(d.BaseDefenseFloor, 0, 15)
	d.RepairBudgetRatio = clamp(d.RepairBudgetRatio, 0, 1)
	d.ScoutReachPriority = clamp(d.ScoutReachPriority, 0, 1)
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// lerp maps a 0–1 doctrine weight to a concrete integer range (e.g. unit cap, cash threshold).
func lerp(min, max int, t float64) int {
	return min + int(math.Round(float64(max-min)*t))
}

func lerpf(min, max, t float64) float64 {
	return min + (max-min)*t
}

// TargetBias holds doctrine-derived multipliers for target scoring.
// Zero values are treated as 1.0 (no adjustment), so the zero-value
// TargetBias preserves existing behavior for all callers and tests.
type TargetBias struct {
	GroundAA     float64 // boost AA targets in BestGroundTarget
	AirGroundDef float64 // boost ground defense targets in BestAirTarget
}

// ComputeTargetBias converts 0-1 doctrine weights into score multipliers.
func ComputeTargetBias(d Doctrine) TargetBias {
	return TargetBias{
		GroundAA:     lerpf(1.0, 4.0, d.GroundTargetAAPriority),
		AirGroundDef: lerpf(1.0, 3.0, d.AirTargetGroundDefPriority),
	}
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
