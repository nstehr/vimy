package rules

import (
	"os/exec"
	"reflect"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"math/rand"
)

// The whole path, in Go: a Doctrine through DoctrineParams, vimyc, and
// LoadArtifact, against what CompileDoctrine produces for the same doctrine.
//
// vimyc's own acceptance test proves the emitted text matches for 45,635 rules.
// It says nothing about this side: whether the parameter names Go sends are the
// ones the rule set declares, whether every action in a full rule set resolves,
// or whether the subprocess plumbing works at all. That is what this covers.
func TestVimycMatchesCompileDoctrine(t *testing.T) {
	if _, err := exec.LookPath("vimyc"); err != nil {
		t.Skip("vimyc not on PATH")
	}
	c, err := NewVimycCompiler("")
	if err != nil {
		t.Fatalf("compiler: %v", err)
	}

	doctrines, err := RealDoctrines()
	if err != nil {
		t.Fatal(err)
	}
	// A subprocess each, so a sample rather than all 500. Fixed seed: a test
	// that fails only some days is worse than one that misses something.
	rng := rand.New(rand.NewSource(7))
	rng.Shuffle(len(doctrines), func(i, j int) {
		doctrines[i], doctrines[j] = doctrines[j], doctrines[i]
	})
	const sample = 12

	rngStates := rand.New(rand.NewSource(11))
	envs := make([]RuleEnv, 0, 60)
	for i := 0; i < 60; i++ {
		envs = append(envs, RuleEnv{
			State: generateState(rngStates), Faction: "soviet", Memory: map[string]any{},
		})
	}

	compared := 0
	for _, d := range doctrines[:sample] {
		mine, err := c.Compile(d)
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		theirs := byName(CompileDoctrine(d))

		if len(mine) != len(theirs) {
			t.Errorf("%s: %d rules, CompileDoctrine gives %d", d.Name, len(mine), len(theirs))
		}
		for _, g := range mine {
			w, ok := theirs[g.Name]
			if !ok {
				t.Errorf("%s: vimyc emitted %q, CompileDoctrine did not", d.Name, g.Name)
				continue
			}
			compared++
			if g.Priority != w.Priority || g.Category != w.Category || g.Exclusive != w.Exclusive {
				t.Errorf("%s/%s: %d %s %t, want %d %s %t", d.Name, g.Name,
					g.Priority, g.Category, g.Exclusive, w.Priority, w.Category, w.Exclusive)
			}
			// Actions are compared by pointer where the registry names one.
			// A factory-built action is a closure, so its identity says
			// nothing; its rendered form does.
			gn, gerr := actionName(g)
			wn, werr := actionName(w)
			if gerr != nil || werr != nil || gn != wn {
				t.Errorf("%s/%s: action %q vs %q (%v, %v)", d.Name, g.Name, gn, wn, gerr, werr)
			}
			if !sameDecision(t, d.Name, g, w, envs) {
				continue
			}
		}
		for name := range theirs {
			found := false
			for _, g := range mine {
				if g.Name == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: CompileDoctrine emitted %q, vimyc did not", d.Name, name)
			}
		}
	}

	if compared < 800 {
		t.Fatalf("only %d rules compared", compared)
	}
	t.Logf("%d rules across %d doctrines agree", compared, sample)
}

// sameDecision compares two conditions by what they decide rather than by their
// text: vimyc writes only the parentheses precedence needs and reprints floats.
func sameDecision(t *testing.T, doctrine string, got, want *Rule, envs []RuleEnv) bool {
	t.Helper()
	compile := func(src string) *vm.Program {
		p, err := expr.Compile(src, expr.Env(RuleEnv{}), expr.AsBool())
		if err != nil {
			t.Errorf("%s/%s: %v\n  %s", doctrine, got.Name, err, src)
			return nil
		}
		return p
	}
	gp, wp := compile(got.ConditionSrc), compile(want.ConditionSrc)
	if gp == nil || wp == nil {
		return false
	}
	for _, env := range envs {
		g, gerr := expr.Run(gp, env)
		w, werr := expr.Run(wp, env)
		if gerr != nil || werr != nil {
			t.Errorf("%s/%s: %v %v", doctrine, got.Name, gerr, werr)
			return false
		}
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s/%s: %v vs %v\n  vimyc: %s\n  go:    %s",
				doctrine, got.Name, g, w, got.ConditionSrc, want.ConditionSrc)
			return false
		}
	}
	return true
}
