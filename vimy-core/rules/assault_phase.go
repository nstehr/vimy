package rules

import (
	"log/slog"
	"math"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/wal"
)

// The base assault is already a state machine; it has just never said so. Its
// state lives in four memory maps — squadAttackState.Attacking, the hunt step,
// and two order throttles — and the transitions are implicit in nested
// conditionals inside SquadAttackKnownBase.
//
// That opacity has cost real mistakes. A reset added for the approach axis
// would silently have undone the widened hunt. A fix to target selection landed
// on a branch that never runs. Neither was visible from any log, because
// nothing records which phase a squad is in.
//
// Naming the phases does not change behaviour. It makes the machine legible
// before anyone decides whether its sequencing is wrong — which is the order
// this project has repeatedly learned to work in.
type assaultPhase string

const (
	// Waiting for enough of the squad to be together to be worth committing.
	phaseRally assaultPhase = "rally"
	// Walking in, possibly via an axis chosen to skirt known defenses.
	phaseApproach assaultPhase = "approach"
	// Close enough to shoot a structure, and doing so.
	phaseStrike assaultPhase = "strike"
	// Arrived and found nothing; searching outward in rings.
	phaseHunt assaultPhase = "hunt"
)

// assaultPhaseEntry is what a squad was doing, and since when.
type assaultPhaseEntry struct {
	Phase assaultPhase
	Since int
	// What the strike phase is shooting at, empty otherwise.
	Target string
}

// recordAssaultPhase notes the squad's phase, logging only transitions — a
// phase that holds for thirty thousand ticks should say so once, not thirty
// thousand times.
func recordAssaultPhase(env RuleEnv, squad string, phase assaultPhase, target string) {
	phases := memoryMap[string, assaultPhaseEntry](env.Memory, "assaultPhase")
	prev, seen := phases[squad]
	if seen && prev.Phase == phase && prev.Target == target {
		return
	}
	held := 0
	if seen {
		held = env.State.Tick - prev.Since
	}
	phases[squad] = assaultPhaseEntry{Phase: phase, Since: env.State.Tick, Target: target}
	slog.Info("assault phase", "squad", squad, "phase", phase,
		"target", target, "tick", env.State.Tick, "previous", prev.Phase, "held_ticks", held)
}

// AssaultPhase reports a squad's current phase, for the dashboard and for
// anyone asking why a squad has been walking for twenty thousand ticks.
func (e RuleEnv) AssaultPhase(squad string) string {
	phases := memoryMap[string, assaultPhaseEntry](e.Memory, "assaultPhase")
	if entry, ok := phases[squad]; ok {
		return string(entry.Phase)
	}
	return ""
}

// Why a squad that was told to attack did not shoot a building.
//
// Game 131 was the first in which the army actually went out: it formed 76
// squads, issued 121 base attacks, and started dying forward rather than at
// home. It also made 34 assault-phase transitions and reached `strike` ZERO
// times, destroying no buildings for the fifth game running.
//
// Four separate things can stop it, with four different fixes, and the phase
// log cannot tell them apart — it records where the squad got to, not what
// turned it back. Counting them was the lesson of the harvester tracker: a
// mechanism guessed at here was wrong once already this session.
const (
	// The squad was too strung out, so it was sent to re-gather and never
	// looked for a target at all.
	StrikeBlockedUnclumped = "unclumped"
	// Nothing of theirs was visible from the squad to score, and the squad had
	// reached the base it remembers. The squad is standing where the enemy is
	// supposed to be and can see nothing: either the memory is wrong, or
	// targeting is blind to buildings it has already scouted. Game 132 hit
	// no-target in 23 of 27 attempts and this is the half that matters.
	StrikeBlockedBlindAtBase = "blind-at-base"
	// Nothing visible, and the squad is still a walk from the base. Expected,
	// and not a defect: it has not arrived yet. Separated from the above
	// because the two have nothing to do with each other and game 132 could
	// not tell them apart — its squad was disengaging under a 5x army deficit,
	// so most of its no-targets may simply have been "nowhere near them".
	StrikeBlockedNoTargetEnRoute = "no-target-en-route"
	// Something was visible and the best of it was a unit, not a building.
	// Mobile units score 1.0 against a building's 3 to 12, so this means no
	// building was in view at all rather than a unit outranking one.
	StrikeBlockedNotBuilding = "not-building"
	// A building was the best target and the squad was still too far from it.
	StrikeBlockedOutOfReach = "out-of-reach"
)

// recordStrikeBlocked tallies one reason a strike did not happen.
//
// Counted per squad per reason, in memory, and read out at game end. Cheap
// enough to run on every evaluation, which matters: the interesting case is
// the one that happens thousands of times.
func recordStrikeBlocked(env RuleEnv, squad, reason string) {
	counts := memoryMap[string, int](env.Memory, "strikeBlocked")
	counts[reason]++
	emit(env, wal.Event{Kind: "strike-blocked", Squad: squad, Reason: reason})
}

// StrikeBlockers reports why strikes did not happen, by reason.
func (e RuleEnv) StrikeBlockers() map[string]int {
	return memoryMap[string, int](e.Memory, "strikeBlocked")
}

// The shape of a squad at the moment it was told to re-gather.
//
// Games 132 and 133 put 110 of 165 failed strikes on `unclumped`, and zero on
// every other cause. So the squad is never assembling — but "the radius is too
// tight" and "the rally cannot reach most of the squad" are different faults
// with different fixes, and the counter cannot tell them apart.
//
// The suspicion is the second. squadIdleActorIDs sends the rally only to IDLE
// members, SquadClumped counts ALL of them, and Idle means actor.IsIdle —
// "has no current order". So issuing the rally makes its recipients non-idle
// and drops them from the next one, while they still count against the 80%.
// If that is right, the idle count here will be a small fraction of members,
// persistently.
//
// Three numbers decide it:
//
//	members ~= idle          the rally reaches everyone; the radius is the fault
//	idle much lower          the rally cannot reach the squad; the predicate is
//	spread very large        the squad is scattered, not merely untidy
type rallyShape struct {
	Rallies    int
	MembersSum int
	IdleSum    int
	SpreadSum  int
}

// Why the squad did or did not route around the defences on its way in.
//
// BestApproachAxis returns false three different ways and true once, and from
// the call site they are indistinguishable - the squad simply walks the direct
// line. Game 172 was toasted entering the base while having observed ONE flame
// tower and one SAM; with that little intel every corridor scores under
// openEnoughThreshold, the mechanism declares the front door already open and
// switches itself off. That is an intel failure, not a routing failure, and
// nothing recorded could tell them apart.
const (
	ApproachNoData        = "no-data"
	ApproachOpen          = "already-open"
	ApproachNoBetterFlank = "no-better-flank"
	ApproachDetour        = "detour"
)

type approachChoices struct{ Counts map[string]int }

// recordApproachChoice counts one approach decision and emits it with the
// corridor scores, so "open because genuinely open" can be separated from
// "open because we have seen nothing".
func recordApproachChoice(env RuleEnv, why string, direct, best float64) {
	emit(env, wal.Event{
		Kind: "approach", Reason: why,
		Attrs: map[string]float64{"direct": direct, "best": best},
	})
	c, _ := env.Memory["approachChoices"].(*approachChoices)
	if c == nil {
		c = &approachChoices{Counts: map[string]int{}}
		env.Memory["approachChoices"] = c
	}
	c.Counts[why]++
}

// ApproachChoices is the per-game tally, for the postmortem.
func (e RuleEnv) ApproachChoices() map[string]int {
	c, _ := e.Memory["approachChoices"].(*approachChoices)
	if c == nil {
		return nil
	}
	return c.Counts
}

// recordRallyShape notes one rally: how many members the squad had, how many
// of them could actually be commanded (the roster minus whoever is retreating
// or held — which is precisely who the rally order is sent to, and never a
// count of idle units, whatever the wire name says), how far the furthest had strayed from
// the centroid, and how many were inside the radius the gate actually measures.
//
// near is the last of those and the only one that answers whether the gate can
// be satisfied. members and spread say how big and how loose; near against
// ceil(0.8n) says whether an 8-cell circle is the thing holding the squad.
// Kept out of the SQLite rally aggregate on purpose — that chain is a
// migration, sqlc, store and Currie for a figure the event stream already
// carries per rally and unsampled.
func recordRallyShape(env RuleEnv, squad string, members, commandable, spread, near int) {
	emit(env, wal.Event{
		Kind: "rally", Squad: squad,
		// Idle is the wire and column name, kept because renaming it would
		// orphan every row already shipped and every WAL segment still on
		// disk. It holds the COMMANDABLE count; see the field's own comment.
		Members: members, Idle: commandable, Spread: spread, Near: near,
	})
	s, _ := env.Memory["rallyShape"].(*rallyShape)
	if s == nil {
		s = &rallyShape{}
		env.Memory["rallyShape"] = s
	}
	s.Rallies++
	s.MembersSum += members
	s.IdleSum += commandable
	s.SpreadSum += spread
}

// squadSpread is the distance from the centroid to the furthest member, in
// cells. Max rather than mean: the 80% rule is about stragglers, and a mean
// hides the one unit holding the whole squad back.
func squadSpread(env RuleEnv, name string, cx, cy int) (members, spread int) {
	squads := getSquads(env.Memory)
	sq, ok := squads[name]
	if !ok {
		return 0, 0
	}
	ids := make(map[int]bool, len(sq.UnitIDs))
	for _, id := range sq.UnitIDs {
		ids[id] = true
	}
	worst := 0
	for _, u := range env.State.Units {
		if !ids[u.ID] {
			continue
		}
		members++
		dx, dy := u.X-cx, u.Y-cy
		if d := dx*dx + dy*dy; d > worst {
			worst = d
		}
	}
	return members, int(math.Sqrt(float64(worst)))
}

// How a squad holds together on the way in.
//
// Game 141 marched the largest army Vimy has fielded at the enemy for 226600
// ticks, traded 1.02, destroyed 19 buildings on the engine's own target
// choices, outscored the winner — and reached `strike` zero times. Everything
// that could stop a strike AT the base has been measured and eliminated:
// target scoring, strike range, and blind targeting all came back zero across
// five games. What is left is that the squad does not arrive together. Its
// spread has sat at 22, 24, 16, 15, 19, 22 cells against a required 8 through
// fixes to the clump arithmetic, the centroid, the order predicate and two
// conscription paths.
//
// The open question is WHERE it comes apart, and the two answers want
// different fixes:
//
//	tight when far, loose when near   they leave together and drift, so
//	                                  something pulls them apart in transit —
//	                                  speed differences, auto-engagement,
//	                                  losses thinning the column
//	loose at every range              they never form up at all, and the
//	                                  rally is not doing its job
//
// Sampled in three bands of distance from the target rather than by journey
// progress, because a squad's journey has no recorded start.
type transitBand struct {
	Samples    int
	MembersSum int
	// Members within the clump radius of the squad's median centre. The
	// diagnostic number: spread says how far the worst straggler is, this says
	// how much of the squad is actually together.
	NearSum   int
	SpreadSum int
}

type transitSpread struct {
	Far  transitBand // beyond 0.35 of the map diagonal from the target
	Mid  transitBand // 0.20 to 0.35
	Near transitBand // inside 0.20, closing on them
}

// recordTransit samples one evaluation of a squad on its way to a target.
func recordTransit(env RuleEnv, name string, tx, ty int) {
	members, near, spread, ok := squadCohesion(env, name)
	if !ok {
		return
	}
	cx, cy, have := squadCentroid(env, name)
	if !have {
		return
	}
	mw, mh := float64(env.State.MapWidth), float64(env.State.MapHeight)
	diag := math.Sqrt(mw*mw + mh*mh)
	if diag <= 0 {
		return
	}
	dx, dy := float64(tx-cx), float64(ty-cy)
	frac := math.Sqrt(dx*dx+dy*dy) / diag

	// The bands below throw `frac` away: 0.34 and 0.21 both become "Mid", and
	// then the Mid samples are summed. Game 148 reduces a 118920-tick game to
	// 76 far / 5 mid / 0 near, which cannot say WHEN the squad stopped
	// closing, only that it did. The event keeps the number.
	emit(env, wal.Event{
		Kind: "transit", Squad: name,
		Members: members, Near: near, Spread: spread,
		Attrs: map[string]float64{"target_fraction": frac},
	})

	t, _ := env.Memory["transitSpread"].(*transitSpread)
	if t == nil {
		t = &transitSpread{}
		env.Memory["transitSpread"] = t
	}
	band := &t.Near
	switch {
	case frac > 0.35:
		band = &t.Far
	case frac > 0.20:
		band = &t.Mid
	}
	band.Samples++
	band.MembersSum += members
	band.NearSum += near
	band.SpreadSum += spread
}

// squadCohesion reports the squad's size, how many sit within squadRallyRadius
// of its median centre, and how far the furthest has strayed.
func squadCohesion(env RuleEnv, name string) (members, near, spread int, ok bool) {
	squads := getSquads(env.Memory)
	sq, found := squads[name]
	if !found || len(sq.UnitIDs) == 0 {
		return 0, 0, 0, false
	}
	ids := make(map[int]bool, len(sq.UnitIDs))
	for _, id := range sq.UnitIDs {
		ids[id] = true
	}
	var present []model.Unit
	for _, u := range env.State.Units {
		if ids[u.ID] {
			present = append(present, u)
		}
	}
	if len(present) < 2 {
		return 0, 0, 0, false
	}
	cx, cy := medianXY(present)
	worst := 0
	for _, u := range present {
		dx, dy := u.X-cx, u.Y-cy
		d := dx*dx + dy*dy
		if d <= squadRallyRadius*squadRallyRadius {
			near++
		}
		if d > worst {
			worst = d
		}
	}
	return len(present), near, int(math.Sqrt(float64(worst))), true
}

// TransitSpread reports how squads held together on the way in.
func (e RuleEnv) TransitSpread() *transitSpread {
	t, _ := e.Memory["transitSpread"].(*transitSpread)
	return t
}
