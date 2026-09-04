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
	return out
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
