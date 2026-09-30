package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// Diagnostics are exported whether or not a rule names them.
//
// The projector is scoped to the questions the compiled rule set asks, which
// silently drops anything we added only to measure. EarnRate was used briefly by
// a savings rule, and when that rule was reverted the scalar stopped appearing:
// game 201 recorded a gross earn rate of zero in every state while earning 110544
// credits, and the export was the only way to see it.
func TestDiagnosticsAreProjectedWithoutARuleNamingThem(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{Tick: 1000},
		Memory: map[string]any{
			"earnRate":     900,
			"earnRatePrev": 700,
		},
	}
	// No rules at all: nothing names anything.
	s := projectFor(env, nil)

	for _, key := range []string{"earn-rate", "earn-rate-prev"} {
		if _, ok := s.Scalars[key]; !ok {
			t.Errorf("%q missing from an export built with no rules; diagnostics must not depend on a rule naming them", key)
		}
	}
	if got := s.Scalars["earn-rate"]; got != 900 {
		t.Errorf("earn-rate = %v, want 900", got)
	}
	if got := s.Scalars["earn-rate-prev"]; got != 700 {
		t.Errorf("earn-rate-prev = %v, want 700", got)
	}
	// And the scoping still holds for everything else.
	if _, ok := s.Scalars["cash"]; ok {
		t.Error("cash was projected with no rule naming it; the scoping should still apply")
	}
}
