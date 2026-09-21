package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/wal"
)

// discard is the cheapest possible sink, to isolate the cost the rule loop
// pays for the call itself rather than for any writing.
type discard struct{}

func (discard) WriteEvent(wal.Event) {}
func (discard) WriteRow(wal.Row)     {}

func benchRules(n int) []*Rule {
	out := make([]*Rule, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, &Rule{
			Name:         "rule-" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Priority:     1000 - i,
			Category:     "cat-" + string(rune('a'+i%7)),
			ConditionSrc: "State.Tick > 0",
			Action:       func(env RuleEnv, conn *ipc.Connection) error { return nil },
		})
	}
	return out
}

// The question the whole design turns on: does streaming every evaluation cost
// the game anything? Measured against a real rule set at the size a doctrine
// compiles to.
func BenchmarkEvaluateWithoutStreaming(b *testing.B) { benchEvaluate(b, false) }
func BenchmarkEvaluateWithStreaming(b *testing.B)    { benchEvaluate(b, true) }

func benchEvaluate(b *testing.B, stream bool) {
	engine, err := NewEngine(benchRules(115))
	if err != nil {
		b.Fatal(err)
	}
	if stream {
		engine.SetEvents(discard{})
	}
	gs := model.GameState{Tick: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := engine.Evaluate(gs, "england", nil); err != nil {
			b.Fatal(err)
		}
	}
}

// And against the real sink, which does a non-blocking channel send per row.
func BenchmarkEvaluateWithLog(b *testing.B) {
	engine, err := NewEngine(benchRules(115))
	if err != nil {
		b.Fatal(err)
	}
	l, err := wal.Open(b.TempDir(), wal.Session{ID: "bench"}, wal.LogOptions{})
	if err != nil {
		b.Fatal(err)
	}
	defer l.Finish(0)
	engine.SetEvents(l)

	gs := model.GameState{Tick: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := engine.Evaluate(gs, "england", nil); err != nil {
			b.Fatal(err)
		}
	}
}
