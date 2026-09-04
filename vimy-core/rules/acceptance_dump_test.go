package rules

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Dumping what CompileDoctrine produces, for vimyc to be held against.
//
// No game needed: CompileDoctrine is a pure function of a Doctrine, so the 500
// archived doctrines are the whole input space worth testing. That is a
// stronger corpus than the recorded rule sets and needs no pairing.

type acceptanceCase struct {
	// Doctrine values by vimyc's spelling, so the other side can bind them
	// directly. Numbers only; the []string preferences are consumed by
	// SetPreferences, never by a rule.
	Params map[string]float64 `json:"params"`
	Rules  []acceptanceRule   `json:"rules"`
}

type acceptanceRule struct {
	Name      string `json:"name"`
	Priority  int    `json:"priority"`
	Category  string `json:"category"`
	Exclusive bool   `json:"exclusive"`
	Action    string `json:"action"`
	Condition string `json:"condition"`
}

// doctrineParams reads a Doctrine's numeric fields by their JSON tags, which
// differ from vimyc's spelling only in the separator.
func doctrineParams(d Doctrine) map[string]float64 {
	out := map[string]float64{}
	v := reflect.ValueOf(d)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.ReplaceAll(tag, "_", "-")
		switch v.Field(i).Kind() {
		case reflect.Float64:
			out[name] = v.Field(i).Float()
		case reflect.Int:
			out[name] = float64(v.Field(i).Int())
		}
	}
	// Facts a rule needs from the []string preference lists. vimyc has no
	// string type and does not need one: what the compiler actually asks of
	// those lists is five yes/no questions, so they cross as 0 or 1 and the
	// lists stay here.
	for name, yes := range map[string]bool{
		"prefers-radar-gated-primary": prefersRadarGatedPrimary(d.PreferredVehicle),
		"prefers-v2-launcher":         c(d).prefersVehicle("v2_launcher"),
		"prefers-artillery":           c(d).prefersVehicle("artillery"),
		"prefers-shock-trooper":       c(d).prefersInfantry("shock_trooper"),
		"prefers-flamethrower":        c(d).prefersInfantry("flamethrower"),
		"specialist-infantry-first":   specialistFirst(d.PreferredInfantry),
		"siege-vehicle-first":         first(d.PreferredVehicle, "v2_launcher", "artillery"),
		"tech-naval-first":            first(d.PreferredNaval, "missile_sub", "cruiser", "destroyer"),
	} {
		if yes {
			out[name] = 1
		} else {
			out[name] = 0
		}
	}
	return out
}

// first reports whether the list's head is one of `want` — the "this is the
// plan" test the compiler applies to the ordered preference lists.
func first(list []string, want ...string) bool {
	if len(list) == 0 {
		return false
	}
	for _, w := range want {
		if list[0] == w {
			return true
		}
	}
	return false
}

func specialistFirst(list []string) bool {
	return first(list, specialistInfantryRoles...)
}

// c is a compiler holding just the doctrine, for the `prefers*` helpers.
func c(d Doctrine) *doctrineCompiler { return &doctrineCompiler{d: d} }

// boundaryDoctrines covers the gate thresholds that real doctrines miss.
//
// The archived 500 are what an LLM actually emits, which clusters: not one has
// an Aggression between 0.3 and 0.4, so the reserve gated on
// `Aggression < DoctrineSignificant` never changes sides and a port could get
// its threshold wrong unnoticed. These sweep every weight across and between
// all six thresholds, and stagger two of them so the differences the compiler
// takes are non-zero.
func boundaryDoctrines() []Doctrine {
	steps := []float64{
		0, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35,
		0.4, 0.45, 0.5, 0.55, 0.6, 0.65, 0.9, 1.0,
	}
	var out []Doctrine
	for _, v := range steps {
		for _, skew := range []float64{0, 0.2} {
			d := Doctrine{
				Name:                      "boundary",
				EconomyPriority:           v,
				Aggression:                v,
				GroundDefensePriority:     v,
				AirDefensePriority:        v,
				TechPriority:              v,
				InfantryWeight:            clamp01(v - skew),
				VehicleWeight:             v,
				AirWeight:                 v,
				NavalWeight:               v,
				ScoutPriority:             v,
				SpecializedInfantryWeight: v,
				SuperweaponPriority:       v,
				CapturePriority:           v,
				TransportAssault:          clamp01(v + skew),
				BaseDefenseFloor:          int(v * 8),
				CommitRatio:               v,
				GroundAttackGroupSize:     4,
				AirAttackGroupSize:        3,
				NavalAttackGroupSize:      3,
			}
			out = append(out, d)
		}
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func TestDumpAcceptanceCorpus(t *testing.T) {
	out := os.Getenv("DUMP_DIR")
	if out == "" {
		t.Skip("needs DUMP_DIR")
	}
	doctrines, err := RealDoctrines()
	if err != nil {
		t.Fatal(err)
	}

	doctrines = append(doctrines, boundaryDoctrines()...)

	cases := make([]acceptanceCase, 0, len(doctrines))
	for _, d := range doctrines {
		c := acceptanceCase{Params: doctrineParams(d)}
		for _, r := range CompileDoctrine(d) {
			action, err := actionName(r)
			if err != nil {
				t.Fatalf("%s: %v", r.Name, err)
			}
			c.Rules = append(c.Rules, acceptanceRule{
				Name:      r.Name,
				Priority:  r.Priority,
				Category:  r.Category,
				Exclusive: r.Exclusive,
				Action:    action,
				Condition: r.ConditionSrc,
			})
		}
		cases = append(cases, c)
	}

	b, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	path := out + "/acceptance.json"
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d doctrines to %s", len(cases), path)
}
