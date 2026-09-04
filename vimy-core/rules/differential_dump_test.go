package rules

import (
	"encoding/json"
	"math/rand"
	"os"
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

type differentialCorpus struct {
	States []vimycState       `json:"states"`
	Cases  []differentialCase `json:"cases"`
}

type differentialCase struct {
	Rule  string `json:"rule"`
	State int    `json:"state"`
	Fired bool   `json:"fired"`
	// Blocked by an exclusive rule in the same category. Go skips these without
	// evaluating, so `fired` is false by definition — but the state is recorded
	// anyway, because "would this have fired had the category been free?" is
	// exactly the counterfactual worth asking, and it cannot be recovered later.
	Skipped bool `json:"skipped"`
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
		corpus.States = append(corpus.States, projectFor(env, DefaultRules()))

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
