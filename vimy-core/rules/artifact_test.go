package rules

import (
	"encoding/json"
	"math/rand"
	"os"
	"reflect"
	"strconv"
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

// Every action the artifact names resolves, and a factory call round trips.
//
// This walked CompileDoctrine's output over 500 doctrines until that compiler
// was deleted. The committed artifact is a real doctrine's rule set, which
// reaches fewer actions but is the shape the loader actually meets.
func TestEveryActionInTheArtifactResolves(t *testing.T) {
	data, err := os.ReadFile("testdata/doctrine_artifact.json")
	if err != nil {
		t.Skipf("no doctrine artifact: %v", err)
	}
	var loaded []artifactRule
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	factories := map[string]bool{}
	for _, a := range loaded {
		if _, err := resolveAction(a.Action); err != nil {
			t.Errorf("%s: %v", a.Name, err)
			continue
		}
		name, args, err := parseActionSrc(a.Action)
		if err != nil {
			t.Errorf("%s: %v", a.Name, err)
			continue
		}
		if args == nil {
			continue
		}
		factories[name] = true
		// A closure's captured arguments cannot be read back, so the check is
		// that rendering the parse reproduces the source exactly.
		as := make([]any, len(args))
		for i, v := range args {
			as[i] = v
		}
		if got := actionSrc(name, as...); got != a.Action {
			t.Errorf("round trip: %q became %q", a.Action, got)
		}
	}
	t.Logf("%d rules, %d factories exercised", len(loaded), len(factories))
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

// `because` reaches the dashboard.
//
// It is the one field in the language that exists purely to be read later, and
// it was being dropped at lowering — parsed, stored, and thrown away before it
// reached Go. The whole chain is short and every link had to be added, so this
// walks all of it.
func TestBecauseReachesTheRuleSummary(t *testing.T) {
	const why = "the first refinery is the whole economy, so it outranks everything but power"
	loaded, err := LoadArtifact([]byte(`[{"name":"build-refinery","priority":750,
	  "category":"economy","exclusive":true,"because":"` + why + `",
	  "action":"produce-refinery","condition":"Cash() >= 1160"}]`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	engine, err := NewEngine(loaded)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	summaries := engine.Rules()
	if len(summaries) != 1 {
		t.Fatalf("%d summaries", len(summaries))
	}
	if summaries[0].Because != why {
		t.Errorf("Because is %q", summaries[0].Because)
	}

	// A rule set that says nothing leaves it empty rather than inventing one.
	quiet, err := LoadArtifact([]byte(`[{"name":"r","priority":1,"category":"economy",
	  "exclusive":false,"action":"scout","condition":"true"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if quiet[0].Because != "" {
		t.Errorf("Because is %q, want empty", quiet[0].Because)
	}
}

// The guard action resolves and takes its arguments in order.
//
// It is the first action written after CompileDoctrine was deleted, so nothing
// generates a corpus containing it — the differential that covered every other
// action cannot see this one.
func TestSquadGuardHarvestersResolves(t *testing.T) {
	fn, err := resolveAction("squad-guard-harvesters(harvester-guard, 0.13)")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if fn == nil {
		t.Fatal("no action")
	}

	name, args, err := parseActionSrc("squad-guard-harvesters(harvester-guard, 0.13)")
	if err != nil {
		t.Fatal(err)
	}
	if name != "squad-guard-harvesters" || len(args) != 2 ||
		args[0] != "harvester-guard" || args[1] != "0.13" {
		t.Errorf("parsed %q %q", name, args)
	}

	// A squad name where the radius belongs must not be accepted.
	if _, err := resolveAction("squad-guard-harvesters(0.13, harvester-guard)"); err == nil {
		t.Error("arguments were accepted in the wrong order")
	}
}

// The dashboard shows the rule set in the language it is written in.
//
// A condition says what the engine tests; the source says it the way someone
// would edit it. Both come from one Ir in one pass of the compiler, so the text
// shown and the text run cannot disagree — this checks the carrying, which is
// the part that can.
func TestSourceReachesTheRuleSummary(t *testing.T) {
	const vy = "rule build-refinery {\n  priority 750\n  category economy exclusive\n" +
		"  do produce-refinery\n  require cash >= 1160\n}"
	loaded, err := LoadArtifact([]byte(`[{"name":"build-refinery","priority":750,
	  "category":"economy","exclusive":true,"action":"produce-refinery",
	  "condition":"Cash() >= 1160","source":` + strconv.Quote(vy) + `}]`))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	engine, err := NewEngine(loaded)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	got := engine.Rules()
	if len(got) != 1 || got[0].Source != vy {
		t.Errorf("Source is %q", got[0].Source)
	}

	// An artifact without one still loads — the dashboard falls back to expr.
	old, err := LoadArtifact([]byte(`[{"name":"r","priority":1,"category":"economy",
	  "exclusive":false,"action":"scout","condition":"true"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if old[0].Source != "" {
		t.Errorf("Source is %q, want empty", old[0].Source)
	}
}
