package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// alwaysRule is a rule whose condition always holds, which is the case share
// exists for: a rule that is permanently eligible monopolises a strict-priority
// exclusive category forever.
func alwaysRule(name string, priority, share int, acted *[]string) *Rule {
	return &Rule{
		Name: name, Priority: priority, Category: "produce-vehicle",
		Exclusive: true, Share: share, ConditionSrc: "true",
		Action: func(env RuleEnv, conn CommandSender) error {
			*acted = append(*acted, name)
			markEffect(env)
			return nil
		},
	}
}

// Without share, the highest-priority always-true rule takes every single turn.
// This is vimy-l6l measured against the engine: produce-scout-vehicle preempted
// 216 times and never run, flak-truck 216, mad-tank 95.
func TestStrictPriorityStarvesTheLoserCompletely(t *testing.T) {
	var acted []string
	eng, err := NewEngine([]*Rule{
		alwaysRule("siege", 485, 0, &acted),
		alwaysRule("tank", 480, 0, &acted),
	})
	if err != nil {
		t.Fatal(err)
	}
	conn, cleanup := testConn(t)
	defer cleanup()
	for i := 0; i < 10; i++ {
		if err := eng.Evaluate(model.GameState{Tick: 1000 + i*100}, "england", conn); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range acted {
		if name == "tank" {
			t.Fatalf("tank should never win a strict-priority category: %v", acted)
		}
	}
	if len(acted) != 10 {
		t.Fatalf("expected 10 wins, got %v", acted)
	}
}

// With share 3, the rationed rule takes one turn in three and the other rule
// gets the rest. This is the whole primitive.
func TestShareRationsTheCategory(t *testing.T) {
	var acted []string
	eng, err := NewEngine([]*Rule{
		alwaysRule("siege", 485, 3, &acted),
		alwaysRule("tank", 480, 0, &acted),
	})
	if err != nil {
		t.Fatal(err)
	}
	conn, cleanup := testConn(t)
	defer cleanup()
	for i := 0; i < 9; i++ {
		if err := eng.Evaluate(model.GameState{Tick: 1000 + i*100}, "england", conn); err != nil {
			t.Fatal(err)
		}
	}
	// Siege wins its first turn (never won before, so unrationed), then stands
	// aside until the category has turned over three times.
	want := []string{"siege", "tank", "tank", "siege", "tank", "tank", "siege", "tank", "tank"}
	if len(acted) != len(want) {
		t.Fatalf("got %v, want %v", acted, want)
	}
	for i := range want {
		if acted[i] != want[i] {
			t.Fatalf("turn %d: got %v, want %v", i, acted, want)
		}
	}
}

// Share 0 and 1 are both unrationed, so every rule in the set today behaves
// exactly as before. This is what makes adding the field a no-op until a rule
// opts in.
func TestShareZeroAndOneAreUnrationed(t *testing.T) {
	for _, share := range []int{0, 1} {
		var acted []string
		eng, err := NewEngine([]*Rule{alwaysRule("siege", 485, share, &acted), alwaysRule("tank", 480, 0, &acted)})
		if err != nil {
			t.Fatal(err)
		}
		conn, cleanup := testConn(t)
		for i := 0; i < 5; i++ {
			if err := eng.Evaluate(model.GameState{Tick: 1000 + i*100}, "england", conn); err != nil {
				t.Fatal(err)
			}
		}
		cleanup()
		for _, n := range acted {
			if n != "siege" {
				t.Errorf("share %d should be unrationed, got %v", share, acted)
				break
			}
		}
	}
}

// Reset clears the rationing clock, so a second game does not inherit the
// first's turn order. The package-level diagnostics in this file predate
// multi-game processes and do not do this; share state lives in Memory for
// exactly that reason.
func TestShareClockResetsBetweenGames(t *testing.T) {
	var acted []string
	eng, err := NewEngine([]*Rule{alwaysRule("siege", 485, 3, &acted), alwaysRule("tank", 480, 0, &acted)})
	if err != nil {
		t.Fatal(err)
	}
	conn, cleanup := testConn(t)
	defer cleanup()
	if err := eng.Evaluate(model.GameState{Tick: 1000}, "england", conn); err != nil {
		t.Fatal(err)
	}
	eng.Reset()
	acted = acted[:0]
	if err := eng.Evaluate(model.GameState{Tick: 2000}, "england", conn); err != nil {
		t.Fatal(err)
	}
	if len(acted) != 1 || acted[0] != "siege" {
		t.Errorf("after Reset the rationed rule should be eligible again, got %v", acted)
	}
}
