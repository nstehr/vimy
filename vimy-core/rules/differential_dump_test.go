package rules

import (
	"encoding/json"
	"math/rand"
	"os"
	"sort"
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

// vimycState mirrors vimyc's `State`. Field names and spellings must match
// exactly — vimyc deserializes with deny_unknown_fields, so a rename here is a
// loud failure there rather than a silent default.
type vimycState struct {
	Cash            int                `json:"cash"`
	PowerExcess     int                `json:"power_excess"`
	BaseUnderAttack bool               `json:"base_under_attack"`
	EnemiesVisible  bool               `json:"enemies_visible"`
	HasEnemyIntel   bool               `json:"has_enemy_intel"`
	NearestEnemy    bool               `json:"nearest_enemy"`
	Units           map[string]int     `json:"units"`
	Buildings       map[string]int     `json:"buildings"`
	Roles           []string           `json:"roles"`
	BuildableRoles  []string           `json:"buildable_roles"`
	QueuesBusy      []string           `json:"queues_busy"`
	QueuesReady     []string           `json:"queues_ready"`
	CanBuild        []string           `json:"can_build"`
	Collections     map[string]int     `json:"collections"`
	SquadReady      map[string]float64 `json:"squad_ready"`
}

// One rule's evaluation, with the state as it stood at that moment.
//
// Per rule rather than per tick because Go's loop is stateful within a tick:
// actions mutate Memory and later rules read it. Projecting once per tick would
// make every squad rule disagree for a reason that is not a bug. See
// vimyc/docs/design.md, "Evaluation semantics".
type differentialCase struct {
	Rule  string     `json:"rule"`
	State vimycState `json:"state"`
	Fired bool       `json:"fired"`
	// Blocked by an exclusive rule in the same category. Go skips these without
	// evaluating, so `fired` is false by definition — but the state is recorded
	// anyway, because "would this have fired had the category been free?" is
	// exactly the counterfactual worth asking, and it cannot be recovered later.
	Skipped bool `json:"skipped"`
}

// The vocabulary vimyc knows. Anything outside this is invisible to it, so
// projecting it would only produce disagreements that are not bugs.
var (
	dumpQueues    = []string{"Building", "Defense", "Vehicle", "Infantry", "Ship", "Aircraft"}
	dumpBuildings = []string{"fact", "powr", "proc", "weap"}
	// Actor types the generator may place. Wider than dumpBuildings on purpose:
	// `barr` is what gives the `barracks` role, without which produce-infantry
	// can never fire and the corpus never exercises it.
	spawnBuildings = []string{"fact", "powr", "proc", "weap", "barr"}
	dumpUnits     = []string{"e1", "mcv"}
	dumpSquads    = []string{"ground-attack", "ground-defense", "air-attack", "naval-attack"}
	dumpColls     = []string{"idle-ground-units", "idle-harvesters", "damaged-buildings"}
)

// kebab converts Go's snake role names to the language's spelling.
func kebab(s string) string { return strings.ReplaceAll(s, "_", "-") }

func project(env RuleEnv) vimycState {
	s := vimycState{
		Cash:            env.Cash(),
		PowerExcess:     env.PowerExcess(),
		BaseUnderAttack: env.BaseUnderAttack(),
		EnemiesVisible:  env.EnemiesVisible(),
		HasEnemyIntel:   env.HasEnemyIntel(),
		NearestEnemy:    env.NearestEnemy() != nil,
		Units:           map[string]int{},
		Buildings:       map[string]int{},
		Collections:     map[string]int{},
		SquadReady:      map[string]float64{},
		Roles:           []string{},
		BuildableRoles:  []string{},
		QueuesBusy:      []string{},
		QueuesReady:     []string{},
		CanBuild:        []string{},
	}

	for _, t := range dumpUnits {
		s.Units[t] = env.UnitCount(t)
	}
	for _, t := range dumpBuildings {
		s.Buildings[t] = env.BuildingCount(t)
	}

	var names []string
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if env.HasRole(name) {
			s.Roles = append(s.Roles, kebab(name))
		}
		if env.CanBuildRole(name) {
			s.BuildableRoles = append(s.BuildableRoles, kebab(name))
		}
	}

	for _, q := range dumpQueues {
		if env.QueueBusy(q) {
			s.QueuesBusy = append(s.QueuesBusy, q)
		}
		if env.QueueReady(q) {
			s.QueuesReady = append(s.QueuesReady, q)
		}
		for _, item := range append(append([]string{}, dumpBuildings...), dumpUnits...) {
			if env.CanBuild(q, item) {
				s.CanBuild = append(s.CanBuild, q+"/"+item)
			}
		}
	}

	s.Collections["idle-ground-units"] = len(env.IdleGroundUnits())
	s.Collections["idle-harvesters"] = len(env.IdleHarvesters())
	s.Collections["damaged-buildings"] = len(env.DamagedBuildings())

	for _, name := range dumpSquads {
		s.SquadReady[name] = env.SquadReadyRatio(name)
	}
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
	var cases []differentialCase

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

		// Mirrors Evaluate's loop, including the exclusivity skip.
		firedCategories := map[string]bool{}
		for _, r := range rules {
			c := differentialCase{Rule: r.Name, State: project(env)}

			if firedCategories[r.Category] {
				c.Skipped = true
				cases = append(cases, c)
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
			cases = append(cases, c)

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

	var lines []string
	for _, c := range cases {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(b))
	}

	path := out + "/differential.jsonl"
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fired, skipped := 0, 0
	for _, c := range cases {
		if c.Skipped {
			skipped++
		} else if c.Fired {
			fired++
		}
	}
	t.Logf("wrote %d cases to %s (%d fired, %d skipped, %d rules)",
		len(cases), path, fired, skipped, len(rules))
}
