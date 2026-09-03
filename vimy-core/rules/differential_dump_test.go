package rules

import (
	"encoding/json"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/expr-lang/expr/vm"
	"github.com/nstehr/vimy/vimy-core/model"
)

// Dumps (state, expected firings) pairs for vimyc's differential test.
//
// Go owns the source of truth: it builds a real GameState, evaluates the seed
// conditions through expr, and *projects* the state down to the flat view vimyc
// understands. Projecting rather than reconstructing matters — the projection
// calls the same RuleEnv methods the conditions do, so the flat state is
// faithful by construction rather than by careful maintenance.
//
//	DUMP_DIR=../../../vimyc/testdata go test -run TestDumpDifferential ./rules/
//
// Not part of the normal suite: it writes files and only exists to feed vimyc.

// vimycState mirrors vimyc's `State`. Keyed by predicate rather than by field,
// so adding a predicate to the manifest needs no change here — see
// vimyc/src/state.rs.
type vimycState struct {
	Scalars     map[string]float64 `json:"scalars"`
	Flags       []string           `json:"flags"`
	Collections map[string]int     `json:"collections"`
	Present     []string           `json:"present"`
	CallsBool   []string           `json:"calls_bool"`
	CallsInt    map[string]int     `json:"calls_int"`
	CallsFloat  map[string]float64 `json:"calls_float"`
	TypeCounts  map[string]int     `json:"type_counts"`
}

// States are stored once and referenced by index: only 400 are distinct, and
// inlining each into all thirteen of its cases made the file 21MB instead of
// under two.
type differentialCorpus struct {
	States []vimycState       `json:"states"`
	Cases  []differentialCase `json:"cases"`
}

type differentialCase struct {
	Rule  string `json:"rule"`
	State int    `json:"state"`
	Fired bool       `json:"fired"`
	// Blocked by an exclusive rule in the same category. Go skips these without
	// evaluating, so `fired` is false by definition — but the state is recorded
	// anyway, because "would this have fired had the category been free?" is
	// exactly the counterfactual worth asking, and it cannot be recovered later.
	Skipped bool `json:"skipped"`
}

// The argument values worth projecting, per domain. A predicate taking a role is
// projected once per role, and so on — the state records answers, so every
// argument a rule might use has to be asked in advance.
var (
	dumpQueues    = []string{"Building", "Defense", "Vehicle", "Infantry", "Ship", "Aircraft"}
	dumpBuildings = []string{"fact", "powr", "proc", "weap"}
	dumpUnits     = []string{"e1", "mcv"}
	dumpSquads    = []string{"ground-attack", "ground-defense", "air-attack", "naval-attack"}
	dumpAxes      = []string{"air", "infantry", "naval", "vehicle"}
	dumpPowers    = []string{"GrantExternalConditionPowerInfoOrder", "NukePowerInfoOrder",
		"SovietParatroopers", "SovietSpyPlane", "UkraineParabombs"}
	// Every float argument any rule passes. Projecting arbitrary floats is
	// impossible, so this is the closed set the compiler actually emits.
	dumpFloats = []float64{0.3, 0.4, 0.5, 0.6, 0.7, 0.75, 0.8, 1.0, 1.5, 2.0}

	// Actor types the generator may place. Wider than dumpBuildings on purpose:
	// `barr` is what gives the `barracks` role, without which produce-infantry
	// can never fire and the corpus never exercises it.
	spawnBuildings = []string{"fact", "powr", "proc", "weap", "barr"}
)

func kebab(s string) string { return strings.ReplaceAll(s, "_", "-") }

func callKey(name string, args ...string) string {
	if len(args) == 0 {
		return name
	}
	return name + "(" + strings.Join(args, ",") + ")"
}

func fmtFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// project asks the env every question a rule could ask, and writes the answers
// down. Driven by reflection over the manifest's shape rather than by a
// hand-written line per predicate, so a new predicate needs no change here.
func project(env RuleEnv) vimycState {
	s := vimycState{
		Scalars: map[string]float64{}, Collections: map[string]int{},
		CallsInt: map[string]int{}, CallsFloat: map[string]float64{},
		TypeCounts: map[string]int{},
		Flags:      []string{}, Present: []string{}, CallsBool: []string{},
	}

	rv := reflect.ValueOf(env)
	// The argument values rules actually pass, per method and position. Asking
	// every value in a domain instead would be millions of calls per corpus —
	// 52 roles times 10 thresholds times 5200 projections — and would answer
	// questions nothing asks. The state records answers, so the right set is
	// exactly the set of questions.
	lits, used := argLiterals()

	for i := 0; i < rv.NumMethod(); i++ {
		mt := rv.Type().Method(i)
		if !used[mt.Name] {
			continue
		}
		ft := mt.Type
		nIn := ft.NumIn() - 1

		var combos [][]string
		var build func(int, []string)
		build = func(p int, acc []string) {
			if p == nIn {
				combos = append(combos, append([]string{}, acc...))
				return
			}
			seen, ok := lits[mt.Name]
			if !ok || p >= len(seen) || len(seen[p]) == 0 {
				return // no rule passes anything here, so nothing to record
			}
			var vals []string
			for v := range seen[p] {
				vals = append(vals, v)
			}
			sort.Strings(vals)
			for _, v := range vals {
				build(p+1, append(acc, v))
			}
		}
		build(0, nil)

		for _, combo := range combos {
			in := make([]reflect.Value, nIn)
			for k, a := range combo {
				if ft.In(k+1).String() == "float64" {
					f, _ := strconv.ParseFloat(a, 64)
					in[k] = reflect.ValueOf(f)
				} else {
					in[k] = reflect.ValueOf(a)
				}
			}
			out := rv.Method(i).Call(in)[0]

			// Roles are snake on this side, kebab in the language.
			wire := make([]string, len(combo))
			for k, a := range combo {
				wire[k] = kebab(a)
			}
			key := callKey(goKebab(mt.Name), wire...)

			switch out.Kind() {
			case reflect.Bool:
				if out.Bool() {
					if nIn == 0 {
						s.Flags = append(s.Flags, key)
					} else {
						s.CallsBool = append(s.CallsBool, key)
					}
				}
			case reflect.Int:
				if nIn == 0 {
					s.Scalars[key] = float64(out.Int())
				} else {
					s.CallsInt[key] = int(out.Int())
				}
			case reflect.Float64:
				if nIn == 0 {
					s.Scalars[key] = out.Float()
				} else {
					s.CallsFloat[key] = out.Float()
				}
			case reflect.Slice:
				s.Collections[key] = out.Len()
			case reflect.Ptr:
				if !out.IsNil() {
					s.Present = append(s.Present, key)
				}
			}
		}
	}

	// `count(e1)` names a type rather than calling a predicate.
	for _, t := range dumpBuildings {
		s.TypeCounts[t] = env.BuildingCount(t)
	}
	for _, t := range dumpUnits {
		s.TypeCounts[t] = env.UnitCount(t)
	}

	sort.Strings(s.Flags)
	sort.Strings(s.Present)
	sort.Strings(s.CallsBool)
	return s
}

// Values chosen to straddle the thresholds the seed rules test. Uniformly
// random states mostly fire nothing; off-by-one disagreements live here.
var (
	cashValues  = []int{0, 99, 100, 101, 299, 300, 301, 1999, 2000, 2001, 5000}
	powerValues = []int{-50, -1, 0, 1, 99, 100, 101}
	countValues = []int{0, 1, 2, 4, 5, 6, 9, 10, 11}
	hpValues    = []int{100, 74, 50}
)

func generateState(rng *rand.Rand) model.GameState {
	pick := func(xs []int) int { return xs[rng.Intn(len(xs))] }

	gs := model.GameState{
		Tick:      rng.Intn(20000),
		MapWidth:  64,
		MapHeight: 64,
	}
	gs.Player.Cash = pick(cashValues)
	gs.Player.PowerProvided = 200
	gs.Player.PowerDrained = 200 - pick(powerValues)

	id := 1
	for _, t := range spawnBuildings {
		if rng.Intn(2) == 0 {
			continue
		}
		hp := pick(hpValues)
		gs.Buildings = append(gs.Buildings, model.Building{
			ID: id, Type: t, X: rng.Intn(64), Y: rng.Intn(64), HP: hp, MaxHP: 100,
		})
		id++
	}

	addUnits := func(t string, n int, idle bool) {
		for i := 0; i < n; i++ {
			gs.Units = append(gs.Units, model.Unit{
				ID: id, Type: t, X: rng.Intn(64), Y: rng.Intn(64),
				HP: 100, MaxHP: 100, Idle: idle,
			})
			id++
		}
	}
	addUnits("e1", pick(countValues), rng.Intn(4) > 0)
	addUnits("mcv", rng.Intn(2), true)
	addUnits("harv", pick(countValues[:4]), rng.Intn(2) == 0)

	for _, q := range dumpQueues {
		if rng.Intn(3) == 0 {
			continue
		}
		item, progress := "", 0
		switch rng.Intn(3) {
		case 1:
			item, progress = "powr", 50
		case 2:
			item, progress = "powr", 100
		}
		// Buildable drives CanBuild, and an empty one makes every `can-build`
		// condition false — which silently left five of the thirteen seed rules
		// unexercised until the coverage check caught it.
		var buildable []string
		for _, candidate := range []string{"powr", "proc", "weap", "barr", "e1", "mcv"} {
			if rng.Intn(2) == 0 {
				buildable = append(buildable, candidate)
			}
		}
		gs.ProductionQueues = append(gs.ProductionQueues, model.ProductionQueue{
			Type: q, CurrentItem: item, CurrentProgress: progress, Buildable: buildable,
		})
	}

	for i := 0; i < rng.Intn(3); i++ {
		gs.Enemies = append(gs.Enemies, model.Enemy{
			ID: id, Type: "e1", X: rng.Intn(64), Y: rng.Intn(64), HP: 100, MaxHP: 100,
		})
		id++
	}
	return gs
}

func TestDumpDifferential(t *testing.T) {
	out := os.Getenv("DUMP_DIR")
	if out == "" {
		t.Skip("no DUMP_DIR")
	}

	rules, err := compileRules(DefaultRules())
	if err != nil {
		t.Fatalf("compile seed rules: %v", err)
	}

	rng := rand.New(rand.NewSource(20260902))
	var corpus differentialCorpus

	for i := 0; i < 400; i++ {
		gs := generateState(rng)
		env := RuleEnv{State: gs, Faction: "soviet", Memory: map[string]any{}}

		// Enemy intel accumulates over ticks, so a one-shot state never has any.
		// Feeding the enemies through updateIntel twice is what lets
		// attack-known-base and scout-with-idle-units both reach true.
		if rng.Intn(2) == 0 {
			seen := gs
			seen.Enemies = append(seen.Enemies, model.Enemy{
				ID: 9001, Type: "barr", X: 60, Y: 60, HP: 100, MaxHP: 100,
			})
			updateIntel(RuleEnv{State: seen, Faction: "soviet", Memory: env.Memory})
		}

		// The same preamble Evaluate runs. HasEnemyIntel and the squad
		// predicates read Memory, so skipping these would make Go and vimyc
		// disagree for a reason that is not a bug.
		updateIntel(env)
		updateBuiltRoles(env)
		updateSquads(env)
		designateScout(env)

		// Projected once, not once per rule. The record stays per rule because
		// that is what a live shadow harness needs, but no action runs here so
		// Memory does not change between rules and all thirteen snapshots would
		// be identical. Recomputing them costs 13x for nothing.
		stateIdx := len(corpus.States)
		corpus.States = append(corpus.States, project(env))

		// Mirrors Evaluate's loop, including the exclusivity skip.
		firedCategories := map[string]bool{}
		for _, r := range rules {
			c := differentialCase{Rule: r.Name, State: stateIdx}

			if firedCategories[r.Category] {
				c.Skipped = true
				corpus.Cases = append(corpus.Cases, c)
				continue
			}

			result, err := vm.Run(r.program, env)
			if err != nil {
				t.Fatalf("rule %q: %v", r.Name, err)
			}
			b, ok := result.(bool)
			if !ok {
				t.Fatalf("rule %q did not return a bool", r.Name)
			}
			c.Fired = b
			corpus.Cases = append(corpus.Cases, c)

			// Actions are not run, which is the point: this answers which
			// rules fire, and therefore which actions *would* run. Executing
			// them would need an ipc.Connection and would send real orders.
			//
			// One consequence to know rather than to fix: with no action
			// running, Memory does not change between rules here, so a tick's
			// snapshots are identical. The per-rule format is still the right
			// one — it is what a live shadow harness needs and costs nothing
			// now — but the intra-tick mutation path is not exercised by this
			// generator.
			if b && r.Exclusive {
				firedCategories[r.Category] = true
			}
		}

	}

	b, err := json.Marshal(corpus)
	if err != nil {
		t.Fatal(err)
	}
	path := out + "/differential.json"
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	fired, skipped := 0, 0
	for _, c := range corpus.Cases {
		if c.Skipped {
			skipped++
		} else if c.Fired {
			fired++
		}
	}
	t.Logf("wrote %d states and %d cases to %s (%d fired, %d skipped, %d rules)",
		len(corpus.States), len(corpus.Cases), path, fired, skipped, len(rules))
}
