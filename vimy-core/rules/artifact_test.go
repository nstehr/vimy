package rules

import (
	"math/rand"
	"os"
	"reflect"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// byName indexes a rule set for comparison.
func byName(rules []*Rule) map[string]*Rule {
	m := make(map[string]*Rule, len(rules))
	for _, r := range rules {
		m[r.Name] = r
	}
	return m
}

// The artifact is the same rule set as DefaultRules, by everything except the
// text of its conditions.
//
// Actions are compared by function pointer, which is the strongest check
// available for the ones the registry names — it is the same function, not
// merely one with the same behaviour.
func TestSeedArtifactMatchesDefaultRules(t *testing.T) {
	loaded, err := SeedRules()
	if err != nil {
		t.Fatalf("load seed artifact: %v", err)
	}

	want := byName(DefaultRules())
	got := byName(loaded)
	if len(got) != len(want) {
		t.Fatalf("artifact has %d rules, DefaultRules has %d", len(got), len(want))
	}

	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("artifact is missing %q", name)
			continue
		}
		if g.Priority != w.Priority {
			t.Errorf("%s: priority %d, want %d", name, g.Priority, w.Priority)
		}
		if g.Category != w.Category {
			t.Errorf("%s: category %q, want %q", name, g.Category, w.Category)
		}
		if g.Exclusive != w.Exclusive {
			t.Errorf("%s: exclusive %t, want %t", name, g.Exclusive, w.Exclusive)
		}
		if reflect.ValueOf(g.Action).Pointer() != reflect.ValueOf(w.Action).Pointer() {
			t.Errorf("%s: resolved a different action", name)
		}
	}
}

// The conditions differ in text — vimyc writes only the parentheses precedence
// needs, and reprints floats — so they are compared by what they decide.
//
// Generated states rather than recorded ones: this is asking whether two
// expressions mean the same thing, and for that, breadth of shapes beats
// fidelity to real play.
func TestSeedArtifactConditionsAgreeWithDefaultRules(t *testing.T) {
	loaded, err := SeedRules()
	if err != nil {
		t.Fatalf("load seed artifact: %v", err)
	}

	type pair struct {
		name      string
		got, want *vm.Program
	}
	var pairs []pair
	want := byName(DefaultRules())
	for _, g := range loaded {
		w := want[g.Name]
		if w == nil {
			t.Fatalf("artifact has %q, DefaultRules does not", g.Name)
		}
		gp, err := expr.Compile(g.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Fatalf("%s: emitted condition does not compile: %v", g.Name, err)
		}
		wp, err := expr.Compile(w.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Fatalf("%s: DefaultRules condition does not compile: %v", g.Name, err)
		}
		pairs = append(pairs, pair{g.Name, gp, wp})
	}

	rng := rand.New(rand.NewSource(1))
	const states = 400
	varied := map[string]bool{}
	for i := 0; i < states; i++ {
		env := RuleEnv{State: generateState(rng), Faction: "soviet", Memory: map[string]any{}}
		for _, p := range pairs {
			g, err := expr.Run(p.got, env)
			if err != nil {
				t.Fatalf("%s: %v", p.name, err)
			}
			w, err := expr.Run(p.want, env)
			if err != nil {
				t.Fatalf("%s: %v", p.name, err)
			}
			if g != w {
				t.Errorf("%s: artifact says %v, DefaultRules says %v", p.name, g, w)
			}
			if g == true {
				varied[p.name] = true
			}
		}
	}

	// Agreement is worthless if nothing was ever true. Not every seed rule can
	// fire against a generated state, but most should.
	if len(varied) < len(pairs)/2 {
		t.Errorf("only %d of %d conditions were ever true over %d states",
			len(varied), len(pairs), states)
	}
}

// Every action the compiler can emit survives being named and read back.
//
// The seed rule set uses none of the factories, so this runs over the 500
// archived doctrines instead — which between them exercise all eleven.
func TestEveryCompiledActionResolves(t *testing.T) {
	seen := map[string]bool{}
	factories := map[string]bool{}

	doctrines, err := RealDoctrines()
	if err != nil {
		t.Fatalf("real doctrines: %v", err)
	}
	for _, d := range doctrines {
		for _, r := range CompileDoctrine(d) {
			src, err := actionName(r)
			if err != nil {
				t.Fatalf("%s: %v", r.Name, err)
			}
			if seen[src] {
				continue
			}
			seen[src] = true

			fn, err := resolveAction(src)
			if err != nil {
				t.Errorf("%q: %v", src, err)
				continue
			}
			name, args, err := parseActionSrc(src)
			if err != nil {
				t.Errorf("%q: %v", src, err)
				continue
			}
			if args == nil {
				// A registry id names a function, so this is the same function.
				if reflect.ValueOf(fn).Pointer() != reflect.ValueOf(r.Action).Pointer() {
					t.Errorf("%q resolved to a different function", src)
				}
				continue
			}
			factories[name] = true

			// A closure's captured arguments cannot be read back, so the check
			// is that rendering the parse reproduces the source exactly.
			as := make([]any, len(args))
			for i, a := range args {
				as[i] = a
			}
			if got := actionSrc(name, as...); got != src {
				t.Errorf("round trip: %q became %q", src, got)
			}
		}
	}

	if len(factories) != len(actionFactories) {
		t.Errorf("only %d of %d factories exercised: %v",
			len(factories), len(actionFactories), factories)
	}
	t.Logf("%d distinct actions, %d factories", len(seen), len(factories))
}

// Which argument goes to which parameter is straight-line code that no
// differential reaches — a swapped domain and role would still parse, still
// resolve, and still round trip. So the order is pinned here.
func TestActionArgumentsParseInOrder(t *testing.T) {
	name, args, err := parseActionSrc("form-squad(ground-attack, Ground, 8, Attack)")
	if err != nil {
		t.Fatal(err)
	}
	if name != "form-squad" {
		t.Errorf("name %q", name)
	}
	want := []string{"ground-attack", "Ground", "8", "Attack"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args %q, want %q", args, want)
	}

	// FormSquad takes (name, domain, size, role); the middle two are the pair
	// that would silently swap.
	if _, err := actionFactories["form-squad"](want); err != nil {
		t.Errorf("form-squad: %v", err)
	}
	if _, err := actionFactories["form-squad"]([]string{"ground-attack", "Ground", "Attack", "8"}); err == nil {
		t.Error("form-squad accepted a size where the role belongs")
	}
}

func TestArtifactRejectsBadActions(t *testing.T) {
	for _, src := range []string{
		"produce-tanks",                                // not in the registry
		"form-squad(ground-attack, Ground)",            // too few arguments
		"form-squad(ground-attack, Ground, x, Attack)", // size is not a number
		"squad-defend()",                               // empty argument list
		"squad-defend(ground-defense",                  // unterminated
		"recall-overextended(ground-attack, )",         // empty argument
	} {
		if _, err := resolveAction(src); err == nil {
			t.Errorf("%q was accepted", src)
		}
	}
}

// A full doctrine's rule set, compiled by vimyc, loads and compiles.
//
// The unit tests above cover the loader on 13 seed rules and the action
// registries on 235 names. This is the thing itself: 101 rules from a real
// doctrine, every condition through expr and every action resolved, which is
// what the engine does at startup and nothing smaller exercises.
func TestDoctrineArtifactLoadsIntoAnEngine(t *testing.T) {
	data, err := os.ReadFile("testdata/doctrine_artifact.json")
	if err != nil {
		t.Skipf("no doctrine artifact: %v", err)
	}
	loaded, err := LoadArtifact(data)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) < 90 {
		t.Fatalf("only %d rules; the artifact looks truncated", len(loaded))
	}
	if _, err := NewEngine(loaded); err != nil {
		t.Fatalf("engine: %v", err)
	}

	// Every rule names a category the engine knows, since a misspelled one
	// silently makes its own exclusivity group.
	for _, r := range loaded {
		if r.Category == "" {
			t.Errorf("%s has no category", r.Name)
		}
	}
	t.Logf("%d rules loaded and compiled", len(loaded))
}
