package rules

import "testing"

// The four reasons a strike does not happen must stay distinguishable, because
// they have four different fixes and the phase log cannot tell them apart.
// Game 131 reached `strike` zero times in 34 transitions and the phase log
// could only say that it had been rallying, approaching and hunting.
func TestStrikeBlockersAreCountedSeparately(t *testing.T) {
	env := RuleEnv{Memory: map[string]any{}}
	recordStrikeBlocked(env, StrikeBlockedUnclumped)
	recordStrikeBlocked(env, StrikeBlockedUnclumped)
	recordStrikeBlocked(env, StrikeBlockedOutOfReach)

	got := env.StrikeBlockers()
	if got[StrikeBlockedUnclumped] != 2 {
		t.Errorf("unclumped = %d, want 2", got[StrikeBlockedUnclumped])
	}
	if got[StrikeBlockedOutOfReach] != 1 {
		t.Errorf("out-of-reach = %d, want 1", got[StrikeBlockedOutOfReach])
	}
	// A reason that never happened must read as absent, not as zero recorded.
	if _, seen := got[StrikeBlockedBlindAtBase]; seen {
		t.Errorf("blind-at-base was never recorded and must not appear")
	}
}
