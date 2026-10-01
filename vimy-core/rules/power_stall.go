package rules

import (
	"strings"

	"github.com/nstehr/vimy/vimy-core/wal"
)

// Instrumenting the power stall, before changing the rule that causes it.
//
// vimy-ccvb: RA sets LowPowerModifier 300, so while power is short everything
// builds three times slower. build-power is gated on `not queue-busy(Building)`,
// so the slowdown it is meant to end is also what holds its own gate shut. Game
// 209 spent 56.7 percent of its sampled states at negative power and never dug
// out: build-power acted 4 times, last at tick 22160 of 32540.
//
// That diagnosis was reconstructed from the 1-in-15 export by evaluating each
// of build-power's clauses separately, which is exactly the kind of inference
// that cost three retracted conclusions in earlier sessions. The stream carries
// `fired` per rule per tick and no reason, so nothing on the wire says WHY a
// rule did not fire. These events say it.
//
// THE ONE QUESTION THEY MUST ANSWER. Two stories fit "power negative, no plant
// arriving", and they want opposite fixes:
//
//	a plant is QUEUED and crawling at 3x  -> the queue gate is correct, and the
//	                                         fix is about placement or preemption
//	a plant is NEVER QUEUED at all        -> the queue gate is the bug, and
//	                                         build-power needs to bypass it
//
// vimy-ccvb asserts the second. plant_queued is the column that can refute it,
// which is the point of measuring before editing the rule.
//
// THE ROLE KEY IS SNAKE HERE. `role-count(power-plant)` in a .vy file compiles
// to RoleCount("power_plant") in Go -- the kebab form is the rule language and
// the export's display spelling, and roles[] is keyed in snake. A miss returns 0
// and false rather than failing, so the dashed spelling would have made every
// count in this file silently zero while the rows kept arriving. That is
// vimy-8wk, and TestPowerStallUsesTheGoSpellingOfTheRole is the guard.
//
// WHAT IS NOT COMPUTED HERE. build-power's cash floor is lerp(500, 200,
// economy-priority), so "could it have afforded one" is doctrine-dependent.
// Rather than bake one doctrine's floor into the wire, the episode rows carry
// counts at BOTH ends of that range and the fraction is a query-time choice --
// the same reason transit carries target_fraction instead of a band. Counts,
// not fractions, so a game that ends mid-episode is still measurable: game 209
// ended at -68 power and an end-of-episode row would never have been written.
const powerEpisodeKey = "powerEpisode"

// powerBeatTicks is the heartbeat cadence while power stays negative, matching
// logCashFlow's 500. An episode that outlives the game still reports.
const powerBeatTicks = 500

// powerEpisode accumulates one continuous run of negative power.
//
// The counters are raw tallies over evaluations, not over ticks: evaluation
// rate is not constant, so these say "in how many of the evaluations we made"
// and a rate per tick is not recoverable from them. samples is the denominator
// for every other count and the only one that may be divided by.
type powerEpisode struct {
	startTick int
	lastBeat  int

	samples     int
	queueBusy   int // the Building queue had an item in progress
	plantQueued int // a power plant was already in production
	canBuild    int // can-build-role(power-plant) was true
	cash200     int // cash >= 200, the floor at economy-priority 1.0
	cash500     int // cash >= 500, the floor at economy-priority 0.0

	// held is the deadlock signature: a plant was buildable and affordable,
	// none was in production, and the Building queue was busy -- so the only
	// clause standing between build-power and firing was its queue gate. One
	// count per cash floor, for the same reason as above.
	heldAt200 int
	heldAt500 int
}

// trackPowerStall runs once per evaluation, before the rule loop, alongside the
// other per-evaluation hooks in Evaluate.
//
// State lives in Memory rather than a package var so that Reset clears it: the
// package-level lastXTick diagnostics predate multi-game processes and would
// carry an episode across games.
func trackPowerStall(env RuleEnv) {
	excess := env.PowerExcess()
	ep, inEpisode := env.Memory[powerEpisodeKey].(*powerEpisode)

	if excess >= 0 {
		if inEpisode {
			emitPowerEvent(env, "power-recovered", ep, excess)
			delete(env.Memory, powerEpisodeKey)
		}
		return
	}

	if !inEpisode {
		ep = &powerEpisode{startTick: env.State.Tick, lastBeat: env.State.Tick}
		env.Memory[powerEpisodeKey] = ep
		emitPowerEvent(env, "power-short", ep, excess)
	}

	cash := env.State.Player.Cash
	busy := env.QueueBusy("Building")
	queued := env.QueueProducingRole("power_plant")
	can := env.CanBuildRole("power_plant")

	ep.samples++
	if busy {
		ep.queueBusy++
	}
	if queued {
		ep.plantQueued++
	}
	if can {
		ep.canBuild++
	}
	if cash >= 200 {
		ep.cash200++
	}
	if cash >= 500 {
		ep.cash500++
	}
	if can && busy && !queued {
		if cash >= 200 {
			ep.heldAt200++
		}
		if cash >= 500 {
			ep.heldAt500++
		}
	}

	if env.State.Tick-ep.lastBeat >= powerBeatTicks {
		emitPowerEvent(env, "power-short-held", ep, excess)
		ep.lastBeat = env.State.Tick
	}
}

// emitPowerEvent writes one episode row. Reason carries what the Building queue
// is working on, because "which building is holding the gate" is the first
// thing to ask of a held episode and item names are few enough for the
// LowCardinality column they land in.
func emitPowerEvent(env RuleEnv, kind string, ep *powerEpisode, excess int) {
	item, progress := buildingQueueItem(env)
	attrs := map[string]float64{
		"power_excess":  float64(excess),
		"cash":          float64(env.State.Player.Cash),
		"plants":        float64(env.RoleCount("power_plant")),
		"bldg_progress": float64(progress),
		"elapsed":       float64(env.State.Tick - ep.startTick),
		"samples":       float64(ep.samples),
	}
	// The instantaneous gates, for the start row that has no history yet. The
	// accumulated counts below are zero on that row and meaningless there.
	attrs["queue_busy"] = boolToFloat(env.QueueBusy("Building"))
	attrs["plant_queued"] = boolToFloat(env.QueueProducingRole("power_plant"))
	attrs["can_build"] = boolToFloat(env.CanBuildRole("power_plant"))

	if ep.samples > 0 {
		attrs["n_queue_busy"] = float64(ep.queueBusy)
		attrs["n_plant_queued"] = float64(ep.plantQueued)
		attrs["n_can_build"] = float64(ep.canBuild)
		attrs["n_cash_200"] = float64(ep.cash200)
		attrs["n_cash_500"] = float64(ep.cash500)
		attrs["n_held_200"] = float64(ep.heldAt200)
		attrs["n_held_500"] = float64(ep.heldAt500)
	}

	emit(env, wal.Event{Kind: kind, Reason: item, Attrs: attrs})
}

// buildingQueueItem is what the Building queue is currently producing, and how
// far along. Progress is the engine's own 0-100, which is the number that makes
// the 3x slowdown visible: an item that holds the gate for 1500 ticks while
// progress crawls is the mechanism, stated.
func buildingQueueItem(env RuleEnv) (string, int) {
	for _, pq := range env.State.ProductionQueues {
		if strings.EqualFold(pq.Type, "Building") {
			return pq.CurrentItem, pq.CurrentProgress
		}
	}
	return "", 0
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
