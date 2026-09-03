package rules

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Dumps the RuleEnv surface for vimyc's env tables.
//
// Reflection gives arity, parameter types and return types; what it cannot give
// is the *domain* of a string parameter — `HasRole(name string)` and
// `QueueBusy(q string)` are identical to it. So this emits everything derivable
// and leaves the domains to be annotated by hand, which is 60-odd short
// decisions rather than 60 hand-transcribed signatures.
//
//	DUMP_DIR=../../../vimyc/testdata go test -run TestDumpEnvManifest ./rules/
//
// Regenerating this is also what would catch Go renaming a method, which
// deriving names in vimyc alone cannot.

type manifestParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type manifestMethod struct {
	Name    string          `json:"name"`
	Kebab   string          `json:"kebab"`
	Params  []manifestParam `json:"params"`
	Results []string        `json:"results"`
	// Best guess at the domain of each string parameter, from the argument
	// literals that actually appear in compiled rules. Empty where the
	// predicate is unused or the literals are ambiguous.
	Domains []string `json:"domains,omitempty"`
	// Whether any rule — seed or doctrine-compiled — actually names it. Half
	// the exported surface is action-only and unusable in a condition, so
	// deciding that here beats maintaining the list on the other side.
	Used bool `json:"used"`
	// Every literal any rule passes at each position, so the other side can
	// check its tables cover what real play produces.
	Literals [][]string `json:"literals,omitempty"`
}

func TestDumpEnvManifest(t *testing.T) {
	out := os.Getenv("DUMP_DIR")
	if out == "" {
		t.Skip("no DUMP_DIR")
	}

	lits, used := argLiterals()
	typ := reflect.TypeOf(RuleEnv{})
	var methods []manifestMethod

	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		ft := m.Type
		mm := manifestMethod{Name: m.Name, Kebab: goKebab(m.Name), Used: used[m.Name]}

		// Field 0 is the receiver.
		for p := 1; p < ft.NumIn(); p++ {
			mm.Params = append(mm.Params, manifestParam{
				Name: "", Type: ft.In(p).String(),
			})
		}
		for r := 0; r < ft.NumOut(); r++ {
			mm.Results = append(mm.Results, ft.Out(r).String())
		}
		for p := range mm.Params {
			d := ""
			vals := []string{}
			if seen, ok := lits[m.Name]; ok && p < len(seen) {
				d = classify(seen[p])
				for v := range seen[p] {
					// Emitted in the language's spelling, not Go's. Roles are
					// snake here and kebab there; this manifest exists for
					// vimyc, so the boundary converts rather than making every
					// consumer know. See vimyc/docs/design.md, "Go adapts to
					// the language".
					if d == "role" {
						v = strings.ReplaceAll(v, "_", "-")
					}
					vals = append(vals, v)
				}
				sort.Strings(vals)
			}
			mm.Domains = append(mm.Domains, d)
			mm.Literals = append(mm.Literals, vals)
		}
		methods = append(methods, mm)
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].Name < methods[j].Name })

	b, err := json.MarshalIndent(methods, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := out + "/env_manifest.json"
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	classified, inUse := 0, 0
	for _, m := range methods {
		if m.Used {
			inUse++
		}
		for _, d := range m.Domains {
			if d != "" {
				classified++
			}
		}
	}
	t.Logf("wrote %d exported methods to %s (%d used by rules, %d string params classified)",
		len(methods), path, inUse, classified)
}
