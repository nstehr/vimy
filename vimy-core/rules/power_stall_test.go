package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// state builds an evaluation where power is short by `short` and the Building
// queue is working on `item`. buildable says whether a power plant is offered,
// which is what CanBuildRole reads.
func powerState(tick, cash, short int, item string, progress int, buildable bool) model.GameState {
	q := model.ProductionQueue{Type: "Building", CurrentItem: item, CurrentProgress: progress}
	if buildable {
		q.Buildable = []string{"powr", "proc"}
	} else {
		q.Buildable = []string{"proc"}
	}
	return model.GameState{
		Tick:             tick,
		Player:           model.Player{Cash: cash, PowerProvided: 100, PowerDrained: 100 + short},
		Buildings:        []model.Building{{ID: 1, Type: "powr"}, {ID: 2, Type: "fact"}},
		ProductionQueues: []model.ProductionQueue{q},
	}
}

// The deadlock signature is recorded while it holds, and the heartbeat carries
// it even if the game never recovers.
//
// vimy-ccvb: build-power is gated on a free Building queue, and low power is
// what keeps that queue busy. Game 209 ran 56.7 percent of its states at
// negative power and ENDED there at -68, so an end-of-episode row would never
// have been written -- which is why the counts ride on the heartbeat too.
func TestPowerStallRecordsTheHeldSignatureWhileItHolds(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	env := func(tick, cash int) RuleEnv {
		return RuleEnv{State: powerState(tick, cash, 50, "proc", 12, true), Memory: mem, Events: sink}
	}

	// Episode opens. A refinery holds the queue, a plant is buildable and
	// affordable, and none is in production: build-power's only failing clause
	// is its own queue gate.
	trackPowerStall(env(1000, 800))
	if len(sink.events) != 1 || sink.events[0].Kind != "power-short" {
		t.Fatalf("a power-negative episode emitted %v; nothing on the wire says why build-power did not fire", sink.events)
	}
	start := sink.events[0]
	if start.Reason != "proc" {
		t.Errorf("reason = %q, want the item holding the Building queue (proc)", start.Reason)
	}
	if start.Attrs["power_excess"] != -50 {
		t.Errorf("power_excess = %v, want -50", start.Attrs["power_excess"])
	}
	if start.Attrs["queue_busy"] != 1 || start.Attrs["can_build"] != 1 || start.Attrs["plant_queued"] != 0 {
		t.Errorf("start gates = busy %v can_build %v plant_queued %v; want 1/1/0",
			start.Attrs["queue_busy"], start.Attrs["can_build"], start.Attrs["plant_queued"])
	}

	// Four more evaluations inside the beat window: counted, not emitted.
	for _, tick := range []int{1100, 1200, 1300, 1400} {
		trackPowerStall(env(tick, 800))
	}
	if len(sink.events) != 1 {
		t.Fatalf("emitted %d rows inside one 500-tick window; the heartbeat is the rate limit", len(sink.events))
	}

	// Past the beat: one heartbeat carrying the accumulated counts.
	trackPowerStall(env(1500, 800))
	if len(sink.events) != 2 || sink.events[1].Kind != "power-short-held" {
		t.Fatalf("after 500 ticks of negative power, events = %v", sink.events)
	}
	beat := sink.events[1]
	if beat.Attrs["samples"] != 6 {
		t.Errorf("samples = %v, want 6", beat.Attrs["samples"])
	}
	// Every sample had the signature, at both ends of build-power's
	// lerp(500, 200, economy-priority) floor.
	if beat.Attrs["n_held_500"] != 6 || beat.Attrs["n_held_200"] != 6 {
		t.Errorf("n_held_500 = %v, n_held_200 = %v, want 6 and 6",
			beat.Attrs["n_held_500"], beat.Attrs["n_held_200"])
	}
	if beat.Attrs["elapsed"] != 500 {
		t.Errorf("elapsed = %v, want 500", beat.Attrs["elapsed"])
	}
}

// A plant already in production is NOT the deadlock, and must not be counted as
// one. This is the column that can refute vimy-ccvb's claim: if plants are
// queued and merely crawling at 3x, the queue gate is correct and the fix lies
// elsewhere.
func TestPowerStallDoesNotCountAQueuedPlantAsHeld(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	for _, tick := range []int{1000, 1100, 1200, 1300, 1400, 1500} {
		// The Building queue is busy with a POWER PLANT: a fix is already on the way.
		st := powerState(tick, 800, 50, "powr", 30, true)
		trackPowerStall(RuleEnv{State: st, Memory: mem, Events: sink})
	}
	beat := sink.events[len(sink.events)-1]
	if beat.Kind != "power-short-held" {
		t.Fatalf("last event = %q, want power-short-held", beat.Kind)
	}
	if beat.Attrs["n_plant_queued"] != 6 {
		t.Errorf("n_plant_queued = %v, want 6 -- a plant in production is the whole distinction", beat.Attrs["n_plant_queued"])
	}
	if beat.Attrs["n_held_500"] != 0 {
		t.Errorf("n_held_500 = %v, want 0: a queued plant is not a held rule", beat.Attrs["n_held_500"])
	}
}

// Recovery closes the episode and forgets it, so the next one starts clean.
func TestPowerStallClosesTheEpisodeOnRecovery(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	trackPowerStall(RuleEnv{State: powerState(1000, 800, 50, "proc", 12, true), Memory: mem, Events: sink})

	// Power restored.
	ok := powerState(2000, 800, -40, "proc", 12, true) // short -40 => 40 to spare
	trackPowerStall(RuleEnv{State: ok, Memory: mem, Events: sink})

	if len(sink.events) != 2 || sink.events[1].Kind != "power-recovered" {
		t.Fatalf("events = %v, want a power-recovered row", sink.events)
	}
	if got := sink.events[1].Attrs["elapsed"]; got != 1000 {
		t.Errorf("elapsed = %v, want 1000", got)
	}
	if _, still := mem[powerEpisodeKey]; still {
		t.Error("episode state survived recovery; the next dip would report one long episode")
	}

	// A healthy base is silent.
	before := len(sink.events)
	trackPowerStall(RuleEnv{State: ok, Memory: mem, Events: sink})
	if len(sink.events) != before {
		t.Errorf("emitted %d rows at positive power; events are for episodes only", len(sink.events)-before)
	}
}

// The role key is snake_case in Go and kebab in the rule language, and a miss
// returns zero rather than failing. role-count(power-plant) in a .vy file is
// RoleCount("power_plant") here, so this guards the exact silent-zero that
// vimy-8wk is about: with a powr standing, plants must not read 0.
func TestPowerStallUsesTheGoSpellingOfTheRole(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	trackPowerStall(RuleEnv{State: powerState(1000, 800, 50, "proc", 12, true), Memory: mem, Events: sink})

	if got := sink.events[0].Attrs["plants"]; got != 1 {
		t.Errorf("plants = %v with one powr standing; a dashed role key reads 0 and says nothing", got)
	}
	if _, ok := roles["power_plant"]; !ok {
		t.Fatal(`roles["power_plant"] is gone: every count in this file silently became 0`)
	}
}

// Cash extremes bracket affordability where the 200/500 counts cannot.
//
// build-power's floor is lerp(500, 200, economy-priority) and economy-priority is
// compiled into the rule, out of Go's reach. Game 210 ran it at 245-305 -- between
// the two brackets -- so the extremes are what settle whether a plant was ever
// affordable during an episode.
func TestPowerStallBracketsAffordabilityWithCashExtremes(t *testing.T) {
	sink := &capture{}
	mem := map[string]any{}
	for i, cash := range []int{480, 120, 300, 90, 700, 250} {
		st := powerState(1000+i*100, cash, 50, "proc", 12, true)
		trackPowerStall(RuleEnv{State: st, Memory: mem, Events: sink})
	}
	// Close the episode so the counts are final.
	trackPowerStall(RuleEnv{State: powerState(2000, 250, -10, "", 0, true), Memory: mem, Events: sink})

	end := sink.events[len(sink.events)-1]
	if end.Kind != "power-recovered" {
		t.Fatalf("last event = %q, want power-recovered", end.Kind)
	}
	if got := end.Attrs["cash_min"]; got != 90 {
		t.Errorf("cash_min = %v, want 90", got)
	}
	if got := end.Attrs["cash_max"]; got != 700 {
		t.Errorf("cash_max = %v, want 700", got)
	}
	// A floor of 260 sits inside [90,700], so the extremes say "sometimes
	// affordable" and the brackets quantify it: 480, 300, 700 and 250 clear 200.
	if got := end.Attrs["n_cash_200"]; got != 4 {
		t.Errorf("n_cash_200 = %v, want 4", got)
	}
	if got := end.Attrs["n_cash_500"]; got != 1 {
		t.Errorf("n_cash_500 = %v, want 1 (only 700)", got)
	}
}
