package rules

import (
	"math"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/wal"
)

// Measuring what a defend-base act accomplishes, before changing how it picks.
//
// WHY. The home-defence response is not missing; it runs hard and loses anyway.
// defend-base acted 226 to 571 times in each of games 205-210, all losses, while
// those games lost 37 to 74 buildings and a seventh to a quarter of their combat
// units died at their OWN base (14-27 percent, against 1.4 and 3.1 percent in the
// two wins). Game 194 won with defend-base never firing once. combat.vy records
// the same shape from game 143: 2143 defensive acts against 248 attacking ones.
//
// So the open question is not whether Vimy reacts but whether reacting works, and
// nothing measures that. stream_rule_evals says defend-base fired; it cannot say
// what happened next. These rows say what an episode of base pressure cost and how
// many acts it absorbed.
//
// THE VALIDITY TERM THAT MATTERS MOST. base-under-attack() is PROXIMITY, not
// damage: any enemy within 20 percent of the map diagonal of any base building
// (env_targets.go:428). It is true in 27-44 percent of the losses, and a loitering
// scout would do that. So an episode that records only its own length proves
// nothing. damaged and lost_buildings are what separate "an enemy is nearby" from
// "the base is being dismantled", and no conclusion should be drawn from an
// episode where both are zero.
//
// WHAT WOULD REFUTE THE FRAMING. If the long episodes turn out to carry no damage
// and no building losses, then the 27-44 percent is loitering, defend-base is
// answering a non-threat hundreds of times, and the finding becomes a predicate
// problem rather than a defence problem -- the mirror of vimy-ho8, where is-rushed
// only fires on damage already taken. Either way it is the same rows that say so.
//
// Episodes, not per-act rows, for the reason the power stall uses them: 561 acts in
// game 210 would be 561 rows against an event stream designed for a few hundred a
// game, and the question is about the episode anyway. Counts rather than fractions,
// and the counts ride on the heartbeat as well as the close, so an episode that is
// still open when the game ends is still measured.
const defensePressureKey = "defensePressure"

// defenseBeatTicks matches the power stall's cadence and logCashFlow's.
const defenseBeatTicks = 500

// defenseEpisode accumulates one continuous run of base pressure.
type defenseEpisode struct {
	startTick int
	lastBeat  int

	samples int

	// What the pressure actually was. critical counts evaluations where core
	// infrastructure was being hit, which is the tier-4 licence in defenders();
	// damaged is the count of evaluations with any damaged building at all.
	critical int
	damaged  int

	// anyDamage is the sensitive end of the ladder: ANY building below full
	// health, where damaged needs a quarter gone and critical needs a critical
	// repair type with an enemy inside 10 cells. Game 211's first episodes read
	// critical 14 against damaged 0 -- a building being shot at that had not yet
	// lost 25 percent -- so reading damaged alone would have filed a real attack
	// as loitering.
	anyDamage int

	// What it cost. Buildings are counted at the episode's start so that losses
	// during it are a subtraction rather than a guess, and the engine's own
	// BuildingsDead is carried too -- it is ground truth and the count of
	// standing buildings is not, since a building under construction appears.
	buildingsAtStart int
	buildingsDeadAt0 int
	unitsAtStart     int

	// The response. acts is defend-base acting, by the tier it had to reach for:
	// the garrison first, then unassigned units, then anything not on the
	// offensive, then the assault itself. Reaching tier 4 means the attack was
	// recalled to save the base, which is the expensive outcome.
	acts         int
	tierGarrison int
	tierUnassign int
	tierReserves int
	tierAssault  int

	// targetChanges counts how often consecutive acts aimed at a DIFFERENT enemy.
	// Game 211 measured 1687 acts across 1889 evaluations under pressure -- 0.89
	// per evaluation, exactly 1.00 in most episodes -- so defend-base re-issues an
	// order roughly every 10 ticks for the whole episode. Whether that is harmful
	// turns entirely on this number: re-sending the SAME attack-move is close to a
	// no-op, while a new target every few ticks is units re-pathing instead of
	// shooting, which is vimy-mfq's harvester oscillation in another costume.
	// Nothing recorded it, so the question could not be settled from game 211.
	lastTargetID  int
	targetChanges int

	// respondersSum and distSum make the means available without shipping a row
	// per act. distSum is in fractions of the map diagonal, so "how far the
	// defenders had to come" is comparable across maps.
	respondersSum int
	distSum       float64
}

// trackDefensePressure runs once per evaluation, beside trackPowerStall.
func trackDefensePressure(env RuleEnv) {
	pressure := env.BaseUnderAttack() || env.CriticalBuildingUnderAttack()
	ep, inEpisode := env.Memory[defensePressureKey].(*defenseEpisode)

	if !pressure {
		if inEpisode {
			emitDefenseEvent(env, "base-pressure-cleared", ep)
			delete(env.Memory, defensePressureKey)
		}
		return
	}

	if !inEpisode {
		ep = &defenseEpisode{
			startTick:        env.State.Tick,
			lastBeat:         env.State.Tick,
			buildingsAtStart: len(env.State.Buildings),
			buildingsDeadAt0: env.State.Player.BuildingsDead,
			unitsAtStart:     len(env.State.Units),
		}
		env.Memory[defensePressureKey] = ep
		emitDefenseEvent(env, "base-pressure", ep)
	}

	ep.samples++
	if env.CriticalBuildingUnderAttack() {
		ep.critical++
	}
	if len(env.DamagedBuildings()) > 0 {
		ep.damaged++
	}
	if anyBuildingDamaged(env) {
		ep.anyDamage++
	}

	if env.State.Tick-ep.lastBeat >= defenseBeatTicks {
		emitDefenseEvent(env, "base-pressure-held", ep)
		ep.lastBeat = env.State.Tick
	}
}

// recordDefendBase is called by ActionDefendBase when it actually sends units, so
// the episode knows what the response was. It emits nothing itself: the act is a
// contribution to the episode, and 561 acts is not 561 rows.
//
// A no-op when no episode is open. defend-base requires base-under-attack() or
// critical-building-under-attack(), the same predicates the tracker keys on, so
// that should not happen -- and if it ever does, the act is deliberately NOT
// counted into a neighbouring episode, because an act outside pressure is a
// different bug and silently folding it in would hide it.
func recordDefendBase(env RuleEnv, tier string, responders []model.Unit, target *model.Enemy) {
	ep, ok := env.Memory[defensePressureKey].(*defenseEpisode)
	if !ok {
		return
	}
	ep.acts++
	if target != nil {
		if ep.acts > 1 && target.ID != ep.lastTargetID {
			ep.targetChanges++
		}
		ep.lastTargetID = target.ID
	}
	ep.respondersSum += len(responders)
	if target != nil {
		ep.distSum += meanJoinFraction(env, responders, target.X, target.Y)
	}
	switch tier {
	case "garrison":
		ep.tierGarrison++
	case "unassigned":
		ep.tierUnassign++
	case "reserves":
		ep.tierReserves++
	case "assault recalled":
		ep.tierAssault++
	}
}

// emitDefenseEvent writes one episode row. Reason carries how deep the threat got
// into the base, bucketed, because the raw distance is a float and `reason` is the
// only string column free here -- squad is left empty rather than repurposed, since
// one column meaning two things is what forced strike_blocked_no_target to be split
// and that split could not be applied backwards.
func emitDefenseEvent(env RuleEnv, kind string, ep *defenseEpisode) {
	attrs := map[string]float64{
		"elapsed": float64(env.State.Tick - ep.startTick),
		"samples": float64(ep.samples),
		"acts":    float64(ep.acts),
	}
	if ep.samples > 0 {
		attrs["n_critical"] = float64(ep.critical)
		attrs["n_damaged"] = float64(ep.damaged)
		attrs["n_any_damage"] = float64(ep.anyDamage)
		attrs["tier_garrison"] = float64(ep.tierGarrison)
		attrs["tier_unassigned"] = float64(ep.tierUnassign)
		attrs["tier_reserves"] = float64(ep.tierReserves)
		attrs["tier_assault"] = float64(ep.tierAssault)
		attrs["buildings_lost"] = float64(env.State.Player.BuildingsDead - ep.buildingsDeadAt0)
		attrs["buildings_delta"] = float64(len(env.State.Buildings) - ep.buildingsAtStart)
		attrs["units_delta"] = float64(len(env.State.Units) - ep.unitsAtStart)
	}
	if ep.acts > 0 {
		attrs["n_target_changes"] = float64(ep.targetChanges)
		attrs["mean_responders"] = float64(ep.respondersSum) / float64(ep.acts)
		attrs["mean_response_fraction"] = ep.distSum / float64(ep.acts)
	}

	emit(env, wal.Event{Kind: kind, Reason: threatDepthBucket(env), Attrs: attrs})
}

// threatDepthBucket says how close the nearest enemy is to the centre of the base,
// as a fraction of the map diagonal, bucketed into names a LowCardinality column
// holds well. base-under-attack() trips at 0.20 of the diagonal from ANY base
// building, so "outskirts" is the band that predicate can fire on while nothing is
// genuinely threatened.
func threatDepthBucket(env RuleEnv) string {
	enemy := env.NearestEnemy()
	if enemy == nil {
		return "no-enemy"
	}
	cx, cy, ok := env.baseCentroid()
	if !ok {
		return "no-base"
	}
	mw, mh := float64(env.State.MapWidth), float64(env.State.MapHeight)
	diag := math.Sqrt(mw*mw + mh*mh)
	if diag == 0 {
		return "no-map"
	}
	frac := math.Hypot(float64(enemy.X-cx), float64(enemy.Y-cy)) / diag
	switch {
	case frac <= 0.05:
		return "inside"
	case frac <= 0.10:
		return "perimeter"
	case frac <= 0.20:
		return "outskirts"
	default:
		return "distant"
	}
}

// anyBuildingDamaged is the most sensitive damage test available: one building
// below full health, of any type, with no proximity requirement. It is the bottom
// rung of the ladder n_critical and n_damaged sit on.
func anyBuildingDamaged(env RuleEnv) bool {
	for _, b := range env.State.Buildings {
		if b.MaxHP > 0 && b.HP < b.MaxHP {
			return true
		}
	}
	return false
}
