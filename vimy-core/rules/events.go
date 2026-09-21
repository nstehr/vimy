package rules

import "github.com/nstehr/vimy/vimy-core/wal"

// Emitting events beside the counters.
//
// The counters stay. They feed the SQLite archive and Currie's per-game
// report, and having both means the stream has an oracle -- a disagreement
// between a counter and a count over the events is a bug in one of them, which
// is worth more than either alone.
//
// What the counters cannot do is carry the unit of analysis. `rally_count` and
// `rally_spread_sum` reduce a game to two integers, so a game with one rally
// and a game with 763 produce means of equal apparent weight; game 146's mean
// spread of 17.0 IS one rally. Pooling at the rally, across games, is the only
// way to see whether a fix moved anything, and that needs the rallies.

// TelemetrySink takes everything the rule loop has to say. Implementations
// must not block: the loop evaluates a rule in ~17us, and a sink that waits
// costs frames.
type TelemetrySink interface {
	WriteEvent(wal.Event)
	WriteRow(wal.Row)
}

// streamEval records one rule evaluation, unsampled.
//
// This is the stream the archive never had. rule_firings counts exactly but
// per doctrine window, with the ticks thrown away; the export carries ticks
// and state but samples 1 in 15 and stops at 20000 cases, which is why
// build-war-factory -- a rule that fires exactly once per game, in 56 of 69
// games -- appears in it zero times. Neither can count at tick resolution.
//
// stateIdx is the export's index when this evaluation happened to be sampled
// for projection, and -1 otherwise. Projecting costs ~60x evaluating, so it
// stays sampled: counting becomes exact while the state-conditioned questions
// stay honestly a sample, and a join on state_idx >= 0 keeps the two apart.
func streamEval(sink TelemetrySink, stateIdx, tick int, ruleSet, rule string, fired, skipped bool) {
	if sink == nil {
		return
	}
	sink.WriteRow(wal.Row{
		Tick: tick, Rule: rule, RuleSet: ruleSet, StateIdx: stateIdx,
		Fired: fired, Skipped: skipped,
	})
}

// emit sends one event if a sink is attached. A nil sink is the normal case in
// tests and when streaming is off.
func emit(env RuleEnv, e wal.Event) {
	if env.Events == nil {
		return
	}
	e.Tick = env.State.Tick
	env.Events.WriteEvent(e)
}
