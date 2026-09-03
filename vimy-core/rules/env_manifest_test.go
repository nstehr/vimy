package rules

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
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
}

var camel = regexp.MustCompile(`([a-z0-9])([A-Z])`)

// goKebab lowercases Go's PascalCase, keeping runs of capitals together.
//
// The second capture needs two or more lowercase letters so that a pluralised
// acronym stays whole: `APCs` is one word, while `AnyCombat` is two.
func goKebab(s string) string {
	s = regexp.MustCompile(`([A-Z]+)([A-Z][a-z]{2,})`).ReplaceAllString(s, "${1}-${2}")
	s = camel.ReplaceAllString(s, "${1}-${2}")
	return strings.ToLower(s)
}

// argLiterals collects, per method, the string literals passed to it across
// every rule the doctrine compiler can emit. That is what turns "param 0 is a
// string" into "param 0 is a role".
func argLiterals() (map[string][]map[string]bool, map[string]bool) {
	out := map[string][]map[string]bool{}
	seen := map[string]bool{}
	call := regexp.MustCompile(`\b([A-Z]\w*)\(([^()]*)\)`)
	lit := regexp.MustCompile(`"([^"]*)"`)
	num := regexp.MustCompile(`^-?\d+(\.\d+)?$`)

	record := func(cond string) {
		for _, m := range call.FindAllStringSubmatch(cond, -1) {
			name, args := m[1], m[2]
			seen[name] = true
			if strings.TrimSpace(args) == "" {
				continue
			}
			parts := strings.Split(args, ",")
			if out[name] == nil {
				out[name] = make([]map[string]bool, 0, len(parts))
			}
			for i, p := range parts {
				for len(out[name]) <= i {
					out[name] = append(out[name], map[string]bool{})
				}
				p = strings.TrimSpace(p)
				if l := lit.FindStringSubmatch(p); l != nil {
					out[name][i][l[1]] = true
				} else if num.MatchString(p) {
					// Unquoted numeric arguments matter too: `SquadReadyRatio`
					// is always called with a literal threshold, and the
					// projector needs to know which ones.
					out[name][i][p] = true
				}
			}
		}
	}

	for _, r := range DefaultRules() {
		record(r.ConditionSrc)
	}
	// Real doctrines rather than synthetic ones: what a predicate is called
	// with depends on which rule blocks the compiler emits, and randomly
	// sampled doctrines emit blocks real play never reaches.
	real, err := RealDoctrines()
	if err != nil {
		panic(err)
	}
	for _, d := range real {
		for _, r := range CompileDoctrine(d) {
			record(r.ConditionSrc)
		}
	}

	return out, seen
}

// classify names the domain a set of literals belongs to, or "" when it cannot
// tell. Deliberately conservative: a wrong guess is worse than a blank, because
// a blank asks to be filled in and a wrong one does not.
func classify(vals map[string]bool) string {
	if len(vals) == 0 {
		return ""
	}
	queues := map[string]bool{"Building": true, "Defense": true, "Vehicle": true,
		"Infantry": true, "Ship": true, "Aircraft": true}
	axes := map[string]bool{"air": true, "infantry": true, "naval": true, "vehicle": true}
	squads := map[string]bool{"ground-attack": true, "ground-defense": true,
		"air-attack": true, "naval-attack": true}
	// OpenRA actor names, split the way vimyc's tables split them. Length is
	// not a usable signal: `e1` is two characters and `fact` is four.
	buildings := map[string]bool{"fact": true, "powr": true, "proc": true, "weap": true}
	units := map[string]bool{"e1": true, "mcv": true}

	all := func(f func(string) bool) bool {
		for v := range vals {
			if !f(v) {
				return false
			}
		}
		return true
	}
	switch {
	case all(func(v string) bool { return queues[v] }):
		return "queue"
	case all(func(v string) bool { return squads[v] }):
		return "squad"
	case all(func(v string) bool { return axes[v] }):
		return "axis"
	case all(func(v string) bool { _, ok := roles[v]; return ok }):
		return "role"
	case all(func(v string) bool { return buildings[v] }):
		return "building"
	case all(func(v string) bool { return units[v] }):
		return "unit"
	case all(func(v string) bool { return buildings[v] || units[v] }):
		// `CanBuild`'s item is a building or a unit depending on the queue.
		return "buildable"
	case all(func(v string) bool { return strings.HasSuffix(v, "InfoOrder") || strings.Contains(v, "Paratroopers") || strings.Contains(v, "SpyPlane") || strings.Contains(v, "Parabombs") }):
		return "support-power"
	}
	return ""
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
			if seen, ok := lits[m.Name]; ok && p < len(seen) {
				d = classify(seen[p])
			}
			mm.Domains = append(mm.Domains, d)
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
