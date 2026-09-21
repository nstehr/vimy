package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/wal"
)

type capture struct {
	events []wal.Event
	rows   []wal.Row
}

func (c *capture) WriteEvent(e wal.Event) { c.events = append(c.events, e) }
func (c *capture) WriteRow(r wal.Row)     { c.rows = append(c.rows, r) }

func envWith(sink TelemetrySink, tick int) RuleEnv {
	return RuleEnv{
		Memory: map[string]any{},
		State:  model.GameState{Tick: tick},
		Events: sink,
	}
}

// The counters stay authoritative; the events are additional. If emitting ever
// replaces counting, Currie's per-game report and the SQLite archive lose the
// numbers they are built on -- and the stream loses its oracle.
func TestStrikeBlockedStillCountsAndNowEmits(t *testing.T) {
	c := &capture{}
	env := envWith(c, 420)
	recordStrikeBlocked(env, "ground-attack", StrikeBlockedUnclumped)

	if got := env.StrikeBlockers()[StrikeBlockedUnclumped]; got != 1 {
		t.Errorf("counter = %d, want 1: the counters must survive", got)
	}
	if len(c.events) != 1 {
		t.Fatalf("emitted %d events, want 1", len(c.events))
	}
	e := c.events[0]
	if e.Kind != "strike-blocked" || e.Reason != StrikeBlockedUnclumped {
		t.Errorf("got kind=%q reason=%q", e.Kind, e.Reason)
	}
	// Tick and squad are what the counters could never carry, and the whole
	// reason for the event: which squad, when.
	if e.Tick != 420 {
		t.Errorf("tick = %d, want 420", e.Tick)
	}
	if e.Squad != "ground-attack" {
		t.Errorf("squad = %q, want the squad it happened to", e.Squad)
	}
}

// A rally's three numbers separate "the radius is too tight" from "the rally
// cannot reach the squad". Summed per game they cannot; per rally they can.
func TestRallyEmitsTheShapeNotJustTheSum(t *testing.T) {
	c := &capture{}
	env := envWith(c, 1000)
	recordRallyShape(env, "ground-attack", 4, 2, 24, 3)
	recordRallyShape(env, "ground-attack", 6, 1, 31, 2)

	if len(c.events) != 2 {
		t.Fatalf("emitted %d events, want one per rally", len(c.events))
	}
	if c.events[0].Members != 4 || c.events[0].Idle != 2 || c.events[0].Spread != 24 {
		t.Errorf("first rally = %+v", c.events[0])
	}
	if c.events[1].Spread != 31 {
		t.Errorf("second rally spread = %d, want 31", c.events[1].Spread)
	}
	// Near is the clump gate's own numerator and the only one of the four that
	// says whether the gate CAN pass. Asserted on the emitted event because the
	// last field added here reached ClickHouse as NULL for a whole game: the
	// sampling, the serialisation and the schema were all correct and two
	// struct literals simply never set it.
	if c.events[0].Near != 3 || c.events[1].Near != 2 {
		t.Errorf("near = %d and %d, want 3 and 2", c.events[0].Near, c.events[1].Near)
	}
	// And the per-game sums still add up, because Currie reads them.
	s, _ := env.Memory["rallyShape"].(*rallyShape)
	if s == nil || s.Rallies != 2 || s.SpreadSum != 55 {
		t.Errorf("counters = %+v, want 2 rallies summing to 55", s)
	}
}

// Streaming is off by default and every existing test runs with a nil sink.
func TestNoSinkIsSafe(t *testing.T) {
	env := RuleEnv{Memory: map[string]any{}, State: model.GameState{Tick: 1}}
	recordStrikeBlocked(env, "ground-attack", StrikeBlockedOutOfReach)
	recordRallyShape(env, "ground-attack", 1, 1, 1, 1)
	if got := env.StrikeBlockers()[StrikeBlockedOutOfReach]; got != 1 {
		t.Errorf("counter = %d, want 1 with no sink attached", got)
	}
}

// The evaluation stream is unsampled: every rule, every tick, including the
// ones the exporter skipped. That is the whole point -- build-war-factory
// fires exactly once per game, and 1-in-15 sampling catches it in none of 69
// games.
func TestEvaluationStreamRecordsEveryRuleEveryTick(t *testing.T) {
	always := &Rule{
		Name: "always", Priority: 100, Category: "a", ConditionSrc: "true",
		Action: func(env RuleEnv, conn *ipc.Connection) error { return nil },
	}
	never := &Rule{
		Name: "never", Priority: 90, Category: "b", ConditionSrc: "false",
		Action: func(env RuleEnv, conn *ipc.Connection) error { return nil },
	}
	engine, err := NewEngine([]*Rule{always, never})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	c := &capture{}
	engine.SetEvents(c)

	for tick := 1; tick <= 4; tick++ {
		if err := engine.Evaluate(model.GameState{Tick: tick}, "england", nil); err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
	}

	if len(c.rows) != 8 { // 2 rules x 4 ticks, nothing sampled away
		t.Fatalf("streamed %d rows, want 8", len(c.rows))
	}
	var firedRows int
	for _, r := range c.rows {
		if r.Fired {
			firedRows++
		}
		// No projection happened, so no state exists to point at. -1 must not
		// be confused with state 0, which is a real index.
		if r.StateIdx != -1 {
			t.Errorf("state_idx = %d with no exporter attached, want -1", r.StateIdx)
		}
		if r.RuleSet == "" {
			t.Error("rule_set is empty: the stream cannot be grouped by what ran")
		}
	}
	if firedRows != 4 {
		t.Errorf("%d rows marked fired, want 4 (one rule, four ticks)", firedRows)
	}
}

// A rule blocked by an exclusive winner in its category is never evaluated.
// Counting those as "declined" is what made the earlier analysis wrong, so the
// stream has to keep them apart.
func TestEvaluationStreamMarksSkippedSeparatelyFromDeclined(t *testing.T) {
	winner := &Rule{
		Name: "winner", Priority: 100, Category: "same", Exclusive: true,
		ConditionSrc: "true",
		Action:       func(env RuleEnv, conn *ipc.Connection) error { return nil },
	}
	loser := &Rule{
		Name: "loser", Priority: 90, Category: "same", ConditionSrc: "true",
		Action: func(env RuleEnv, conn *ipc.Connection) error { return nil },
	}
	engine, err := NewEngine([]*Rule{winner, loser})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	c := &capture{}
	engine.SetEvents(c)
	if err := engine.Evaluate(model.GameState{Tick: 1}, "england", nil); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	byRule := map[string]wal.Row{}
	for _, r := range c.rows {
		byRule[r.Rule] = r
	}
	if !byRule["winner"].Fired {
		t.Error("winner should have fired")
	}
	if byRule["loser"].Fired {
		t.Error("loser must not read as fired")
	}
	if !byRule["loser"].Skipped {
		t.Error("loser lost its exclusive slot and must read as skipped, not as declined")
	}
}

// The fingerprint is cached because it is now read every tick. It must follow
// a swap, or every row after one is labelled with the rule set that is gone.
func TestRuleSetIDFollowsASwap(t *testing.T) {
	first := &Rule{
		Name: "a", Priority: 100, Category: "x", ConditionSrc: "true",
		Action: func(env RuleEnv, conn *ipc.Connection) error { return nil },
	}
	engine, err := NewEngine([]*Rule{first})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	c := &capture{}
	engine.SetEvents(c)
	if err := engine.Evaluate(model.GameState{Tick: 1}, "england", nil); err != nil {
		t.Fatal(err)
	}
	before := c.rows[len(c.rows)-1].RuleSet

	second := &Rule{
		Name: "a", Priority: 100, Category: "x", ConditionSrc: "false",
		Action: func(env RuleEnv, conn *ipc.Connection) error { return nil },
	}
	if err := engine.Swap([]*Rule{second}); err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if err := engine.Evaluate(model.GameState{Tick: 2}, "england", nil); err != nil {
		t.Fatal(err)
	}
	after := c.rows[len(c.rows)-1].RuleSet

	if before == after {
		t.Errorf("rule_set is %q before and after a swap; the cache is stale", before)
	}
	if want := RuleSetID([]*Rule{second}); after != want {
		t.Errorf("rule_set = %q after the swap, want %q", after, want)
	}
}

// The transit sampler had the most to lose from a counter: it computes a
// continuous distance and then buckets it into three bands and sums them, so
// game 148's whole 118920 ticks reduce to 76 far / 5 mid / 0 near. That cannot
// say WHEN the squad stopped closing, and it bakes the band boundaries into
// storage so a threshold change cannot be evaluated against old games.
func TestTransitEmitsTheDistanceItOtherwiseBuckets(t *testing.T) {
	c := &capture{}
	env := envWith(c, 5000)
	env.State.MapWidth, env.State.MapHeight = 100, 100

	// getSquads returns a throwaway map when the key is absent, so the squad
	// has to go into Memory directly.
	env.Memory["squads"] = map[string]*Squad{
		"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{1, 2, 3}},
	}
	env.State.Units = []model.Unit{
		{ID: 1, Type: "e1", X: 10, Y: 10},
		{ID: 2, Type: "e1", X: 12, Y: 12},
		{ID: 3, Type: "e1", X: 40, Y: 40},
	}
	recordTransit(env, "ground-attack", 90, 90)

	if len(c.events) != 1 {
		t.Fatalf("emitted %d events, want 1", len(c.events))
	}
	e := c.events[0]
	if e.Kind != "transit" || e.Squad != "ground-attack" || e.Tick != 5000 {
		t.Errorf("got %+v", e)
	}
	frac, ok := e.Attrs["target_fraction"]
	if !ok {
		t.Fatal("target_fraction missing: the whole point of the row is the distance the bands discard")
	}
	if frac <= 0 || frac >= 1 {
		t.Errorf("target_fraction = %v, want a fraction of the map diagonal", frac)
	}
	// near is the subset within the radius of the centre, NOT the subset an
	// order can reach. Sharing rally's `idle` field would make one column mean
	// two things.
	if e.Near == 0 && e.Members == 0 {
		t.Error("cohesion was not carried")
	}
	if e.Idle != 0 {
		t.Errorf("idle = %d on a transit row; it is rally's measurement", e.Idle)
	}

	// And the in-memory bands still accumulate, because Currie reads them.
	ts, _ := env.Memory["transitSpread"].(*transitSpread)
	if ts == nil || ts.Far.Samples+ts.Mid.Samples+ts.Near.Samples != 1 {
		t.Errorf("the band counters must survive: %+v", ts)
	}
}

// The engine is one object and connections are many: the mod's bot module is a
// per-player trait with its own socket, and a game that did not close cleanly
// leaves one behind. Observed on the first real run -- two sessions in the same
// second, one of them empty. A second claim must be refused, not silently win.
func TestAttachEventsRefusesASecondClaim(t *testing.T) {
	engine, err := NewEngine(DefaultRules())
	if err != nil {
		t.Fatal(err)
	}
	first := &capture{}
	got, err := engine.AttachEvents(func() (TelemetrySink, error) { return first, nil })
	if err != nil || got == nil {
		t.Fatalf("first claim: got %v, err %v", got, err)
	}

	built := false
	second, err := engine.AttachEvents(func() (TelemetrySink, error) {
		built = true
		return &capture{}, nil
	})
	if err != nil {
		t.Fatalf("second claim errored: %v", err)
	}
	if second != nil {
		t.Error("second claim succeeded; the first game's telemetry would stop mid-game")
	}
	// Nothing may be built for a losing claim -- building opens a session
	// directory, and an empty one ships as a game_id 0 row.
	if built {
		t.Error("the losing claim built a sink; it would leave an empty session behind")
	}

	if err := engine.Evaluate(model.GameState{Tick: 1}, "england", nil); err != nil {
		t.Fatal(err)
	}
	if len(first.rows) == 0 {
		t.Error("the holder stopped receiving rows")
	}
}

// And the claim must be released at game end, or the second game of a session
// streams nothing.
func TestDetachEventsFreesTheClaim(t *testing.T) {
	engine, err := NewEngine(DefaultRules())
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := engine.AttachEvents(func() (TelemetrySink, error) { return &capture{}, nil }); got == nil {
		t.Fatal("first claim failed")
	}
	engine.DetachEvents()

	next := &capture{}
	got, err := engine.AttachEvents(func() (TelemetrySink, error) { return next, nil })
	if err != nil || got == nil {
		t.Fatalf("the next game could not claim the sink: got %v, err %v", got, err)
	}
	if err := engine.Evaluate(model.GameState{Tick: 1}, "england", nil); err != nil {
		t.Fatal(err)
	}
	if len(next.rows) == 0 {
		t.Error("the new holder receives nothing")
	}
}
