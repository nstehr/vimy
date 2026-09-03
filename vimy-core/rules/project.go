package rules

// Projecting a RuleEnv into the flat state vimyc's evaluator consumes.
//
// Lives in the package rather than in a test because two consumers need it and
// two implementations would drift: the offline dump that builds vimyc's
// differential corpus, and the live exporter that records real games.
//
// Costs about 1ms against 17us to evaluate the seed rules, so the live exporter
// samples rather than recording every evaluation.

import (
	"math/rand"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/nstehr/vimy/vimy-core/model"
)

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

// normaliseLiteral renders an argument the way both sides agree on: roles kebab,
// numbers in their shortest exact form.
func normaliseLiteral(s string) string {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	return kebab(s)
}

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
var (
	projArgsOnce sync.Once
	projLits     map[string][]map[string]bool
	projUsed     map[string]bool
)

// projectionArgs covers everything any doctrine can emit, which is what the
// offline dump wants. A live recording should use `conditionArgs` over the rule
// set actually loaded instead — the union asks about 38 different
// `overextended-squad-members` thresholds when the active rules use two.
func projectionArgs() (map[string][]map[string]bool, map[string]bool) {
	projArgsOnce.Do(func() { projLits, projUsed = argLiterals() })
	return projLits, projUsed
}

// recordCallArgs notes, per method and argument position, every literal a
// condition passes. Shared so a live recording and the offline dump agree on
// what a projection should ask.
func recordCallArgs(cond string, out map[string][]map[string]bool, seen map[string]bool) {
	call := regexp.MustCompile(`\b([A-Z]\w*)\(([^()]*)\)`)
	lit := regexp.MustCompile(`"([^"]*)"`)
	num := regexp.MustCompile(`^-?\d+(\.\d+)?$`)

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
				out[name][i][p] = true
			}
		}
	}
}

// conditionArgs collects the literals a specific rule set passes, so a
// projection asks only the questions those rules ask.
func conditionArgs(rules []*Rule) (map[string][]map[string]bool, map[string]bool) {
	out := map[string][]map[string]bool{}
	seen := map[string]bool{}
	for _, r := range rules {
		recordCallArgs(r.ConditionSrc, out, seen)
	}
	return out, seen
}

// project asks the env every question the given rules could ask, and writes the
// answers down. Nil rules means every question any doctrine could ask.
func projectFor(env RuleEnv, rules []*Rule) vimycState {
	if rules == nil {
		return project(env)
	}
	lits, used := conditionArgs(rules)
	return projectWith(env, lits, used)
}

func project(env RuleEnv) vimycState {
	lits, used := projectionArgs()
	return projectWith(env, lits, used)
}

func projectWith(env RuleEnv, lits map[string][]map[string]bool, used map[string]bool) vimycState {
	s := vimycState{
		Scalars: map[string]float64{}, Collections: map[string]int{},
		CallsInt: map[string]int{}, CallsFloat: map[string]float64{},
		TypeCounts: map[string]int{},
		Flags:      []string{}, Present: []string{}, CallsBool: []string{},
	}

	rv := reflect.ValueOf(env)
	// The argument values rules actually pass, per method and position.
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

			// Roles are snake on this side, kebab in the language. Numbers
			// are normalised, because these come from the literal text in a
			// condition while the other side has parsed them: Go's `0.10` and
			// vimyc's `0.1` are the same threshold and must key the same.
			wire := make([]string, len(combo))
			for k, a := range combo {
				wire[k] = normaliseLiteral(a)
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
	record := func(cond string) { recordCallArgs(cond, out, seen) }

	// The seed rules too: the engine runs them before the first doctrine swap,
	// and they are the only user of `HasBuilding` and `BuildingCount`.
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
	case all(func(v string) bool {
		return strings.HasSuffix(v, "InfoOrder") || strings.Contains(v, "Paratroopers") || strings.Contains(v, "SpyPlane") || strings.Contains(v, "Parabombs")
	}):
		return "support-power"
	}
	return ""
}
