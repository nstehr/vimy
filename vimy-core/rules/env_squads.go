package rules

import (
	"math"
	"slices"

	"github.com/nstehr/vimy/vimy-core/model"
)

// SquadClumped reports whether enough living members are within radiusCells of
// the centroid. Gates squad-attack on the squad having actually assembled: a
// strung-out squad is picked off one unit at a time.
//
// "Enough" is 80% rounded DOWN, which is not what the obvious integer test
// gives. near*10 >= len*8 rounds up, and for a squad of four that demands
// 4 of 4 — for two or three it demands all of them too. Vimy's squads
// averaged 3.5 members at the moment they were told to re-gather, so the gate
// it was actually applying was "every single unit within 8 cells", and one
// straggler anywhere blocked the assault permanently. Games 132 and 133 put
// 110 of 165 failed strikes on this check and reached `strike` zero times.
//
// So the rule is 80%, capped at one straggler's worth of slack: a squad of
// four needs three, of six needs five, of ten needs eight. A squad of two
// still needs both — at that size there is no minority, and a lone unit
// walking into a base is the piecemeal death this check exists to prevent.
func (e RuleEnv) SquadClumped(name string, radiusCells int) bool {
	_, near, need := e.SquadClump(name, radiusCells)
	return near >= need
}

// SquadClump is the gate's own arithmetic, exposed so that a strike blocked on
// dispersion can record WHY rather than only that it was. near is how many
// members sit within radiusCells of their median and need is what that has to
// reach; SquadClumped is near >= need and nothing more.
//
// Split out because the rally instrumentation recorded the furthest straggler,
// which is a maximum, while the gate reads a percentile. A squad of 31 whose
// furthest member is 46 cells out is equally consistent with 25 packed tight
// and 6 trailing — which passes — and with 31 evenly strung out, which cannot.
// Game 154 blocked 80 strikes here and could not distinguish the two, so it
// could not say whether the fixed 8-cell radius is the binding constraint once
// a squad is army-sized.
func (e RuleEnv) SquadClump(name string, radiusCells int) (members, near, need int) {
	squads, ok := e.Memory["squads"].(map[string]*Squad)
	if !ok {
		return 0, 0, 0 // no squad map — trivially "clumped"
	}
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return 0, 0, 0 // no squad — no dispersion to worry about
	}

	ids := make(map[int]bool, len(sq.UnitIDs))
	for _, id := range sq.UnitIDs {
		ids[id] = true
	}
	var alive []model.Unit
	for _, u := range e.State.Units {
		if !ids[u.ID] {
			continue
		}
		alive = append(alive, u)
	}
	if len(alive) < 2 {
		// One unit is trivially clumped, and a need of zero says so.
		return len(alive), len(alive), 0
	}
	// Median, not mean. A mean is dragged by the very stragglers this check is
	// meant to tolerate: three units at x=50, 50 and 100 have a mean of 66, so
	// the two standing together are 16 cells from their own "centre" and the
	// squad scores ZERO within an 8-cell radius. Allowing a straggler is
	// meaningless while one straggler can disqualify everybody. The median
	// sits with the cluster and ignores the outlier, which is the question
	// being asked: are most of them together?
	cx, cy := medianXY(alive)

	radiusSq := radiusCells * radiusCells
	for _, u := range alive {
		dx := u.X - cx
		dy := u.Y - cy
		if dx*dx+dy*dy <= radiusSq {
			near++
		}
	}
	// The 80% figure, but never so strict that it demands everyone: one unit
	// may always lag. Plain flooring was the first attempt and drifts too far
	// the other way — it would let two of six straggle, where 80% permits one.
	need = (len(alive)*8 + 9) / 10 // ceil(0.8n)
	if max := len(alive) - 1; need > max {
		need = max
	}
	if need < 2 {
		need = 2
	}
	return len(alive), near, need
}

// medianXY is the per-axis median position of a set of units. Per-axis rather
// than a true geometric median: it is cheap, robust to outliers, and lands
// inside the cluster, which is all this needs.
func medianXY(units []model.Unit) (int, int) {
	xs := make([]int, len(units))
	ys := make([]int, len(units))
	for i, u := range units {
		xs[i], ys[i] = u.X, u.Y
	}
	slices.Sort(xs)
	slices.Sort(ys)
	return xs[len(xs)/2], ys[len(ys)/2]
}

// AxisBurned reports an axis the doctrine has repeatedly pivoted to and been
// countered on. A hard gate rather than a prompt constraint: told only in the
// prompt, the LLM kept committing aircraft into flak.
func (e RuleEnv) AxisBurned(axis string) bool {
	return slices.Contains(e.Signals.BurnedAxes, axis)
}

// OverextendedSquadMembers returns idle squad members beyond leashPct of the
// map diagonal that are also not making forward progress — closer to our base
// than to any known enemy base.
//
// The progress test is what keeps recall from looping: units staging at a flank
// waypoint, in transit, or fighting at the enemy base all sit far from home and
// arrive idle. Recalled, they re-form, get sent out again, and arrive idle again.
// A fixed radius around the enemy base was too narrow to cover flank staging.
func (e RuleEnv) OverextendedSquadMembers(name string, leashPct float64) []model.Unit {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return nil
	}
	centX, centY := e.BuildingCentroid()
	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	diagonal := math.Sqrt(mw*mw + mh*mh)
	leashDist := diagonal * leashPct
	leashSq := leashDist * leashDist
	bases := getEnemyBases(e.Memory)

	idleSet := make(map[int]bool)
	unitMap := make(map[int]model.Unit)
	for _, u := range e.State.Units {
		unitMap[u.ID] = u
		if u.Idle {
			idleSet[u.ID] = true
		}
	}

	var out []model.Unit
	for _, id := range sq.UnitIDs {
		if !idleSet[id] {
			continue
		}
		u := unitMap[id]
		dx := float64(u.X - centX)
		dy := float64(u.Y - centY)
		distToHomeSq := dx*dx + dy*dy
		if distToHomeSq <= leashSq {
			continue
		}
		// Closer to the enemy than to home means in transit or staging.
		forwardProgressing := false
		for _, b := range bases {
			ex := float64(u.X - b.X)
			ey := float64(u.Y - b.Y)
			if ex*ex+ey*ey < distToHomeSq {
				forwardProgressing = true
				break
			}
		}
		if forwardProgressing {
			continue
		}
		out = append(out, u)
	}
	return out
}

// SquadThreatRatio is local enemy HP over squad HP; above 1.0 we are outmatched
// where we stand. radiusPct is a fraction of the map diagonal.
func (e RuleEnv) SquadThreatRatio(name string, radiusPct float64) float64 {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return 0
	}
	unitMap := make(map[int]model.Unit)
	for _, u := range e.State.Units {
		unitMap[u.ID] = u
	}
	sumX, sumY, squadHP, n := 0, 0, 0, 0
	for _, id := range sq.UnitIDs {
		if u, ok := unitMap[id]; ok {
			sumX += u.X
			sumY += u.Y
			squadHP += u.HP
			n++
		}
	}
	if n == 0 || squadHP == 0 {
		return 0
	}
	cx, cy := sumX/n, sumY/n

	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	radius := math.Sqrt(mw*mw+mh*mh) * radiusPct
	radiusSq := radius * radius

	enemyHP := 0
	for _, en := range e.State.Enemies {
		// Unarmed structures are the squad's OBJECTIVE, not its danger. The mod
		// includes buildings in the enemy list on purpose, so a squad sent to
		// attack a base counted the base as the force opposing it: game 94 read
		// a median threat ratio of 7.96 and a p90 of 21 while engaged, and
		// squad-disengage — which fires above about 2.5 — decided to withdraw
		// 31 times against 15 attacks. It was retreating from what it came to
		// destroy. Defensive structures still count; a pillbox is a real reason
		// to leave.
		if IsUnarmedStructure(en.Type) {
			continue
		}
		dx := float64(en.X - cx)
		dy := float64(en.Y - cy)
		if dx*dx+dy*dy <= radiusSq {
			enemyHP += en.HP
		}
	}
	if enemyHP == 0 {
		return 0
	}
	return float64(enemyHP) / float64(squadHP)
}

func (e RuleEnv) SquadExists(name string) bool {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	return ok && len(sq.UnitIDs) > 0
}

func (e RuleEnv) SquadSize(name string) int {
	squads := getSquads(e.Memory)
	if sq, ok := squads[name]; ok {
		return len(sq.UnitIDs)
	}
	return 0
}

func (e RuleEnv) SquadNeedsReinforcement(name string) bool {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return false
	}
	return len(sq.UnitIDs) < sq.TargetSize
}

// SquadReadyRatio is how much of the INTENDED force is alive and present:
// living members over TargetSize, capped at 1.
//
// It counted members flagged Idle, and Idle means "has no current order" — it
// clears the instant the sidecar issues one. So a squad marching on its target,
// doing exactly what it was told, read as zero ready, while a single stranded
// unit with nothing to do read as 1.0. The gate was inverted: too strict for a
// real assault and too lax for a remnant.
//
// That inversion was worked around twice rather than fixed — activation() was
// clamped to 0.5 in "Ask the squad for a readiness it can actually reach" and
// then to 0.25 in "ask the squad for what exists" — which made commit_ratio
// inert above 0.25. The strategist asks for 0.70 to avoid piecemeal waves, the
// prompt tells it to ask for 0.6-0.8 against a fortified base, and none of it
// could bind. squad-attack's own note called the clamp "a calibration and not a
// cure", and said the cure was that readiness must not mean standing still.
//
// Game 158 is what the stopgap cost: the squad decayed 15 to 6 to 2 to 1 and
// kept walking into the enemy base, because a remnant of one scores full marks.
// Army peak fell to 5000 against game 155's 11800 — the force never
// accumulated, it was committed and lost and committed again.
//
// Retreating units are excluded from the numerator but NOT from TargetSize: a
// squad half of which is limping to the depot is not ready to assault, which is
// the question being asked.
func (e RuleEnv) SquadReadyRatio(name string) float64 {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return 0
	}
	target := sq.TargetSize
	if target <= 0 {
		// No target recorded: fall back to the roster so the gate stays a
		// fraction of something real rather than dividing by zero.
		target = len(sq.UnitIDs)
	}
	alive := make(map[int]bool, len(e.State.Units))
	for _, u := range e.State.Units {
		alive[u.ID] = true
	}
	retreating := getRetreatingUnits(e.Memory)
	present := 0
	for _, id := range sq.UnitIDs {
		if _, isRetreating := retreating[id]; isRetreating {
			continue
		}
		if alive[id] {
			present++
		}
	}
	if present >= target {
		return 1
	}
	return float64(present) / float64(target)
}

func (e RuleEnv) SquadIdleCount(name string) int {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok {
		return 0
	}
	idleSet := make(map[int]bool)
	for _, u := range e.State.Units {
		if u.Idle {
			idleSet[u.ID] = true
		}
	}
	retreating := getRetreatingUnits(e.Memory)
	n := 0
	for _, id := range sq.UnitIDs {
		_, isRetreating := retreating[id]
		if idleSet[id] && !isRetreating {
			n++
		}
	}
	return n
}

// SquadAwayFromBase keeps disengage from firing on a squad already at home.
// radiusPct is a fraction of the map diagonal.
func (e RuleEnv) SquadAwayFromBase(name string, radiusPct float64) bool {
	squads := getSquads(e.Memory)
	sq, ok := squads[name]
	if !ok || len(sq.UnitIDs) == 0 {
		return false
	}
	unitMap := make(map[int]model.Unit)
	for _, u := range e.State.Units {
		unitMap[u.ID] = u
	}
	sumX, sumY, n := 0, 0, 0
	for _, id := range sq.UnitIDs {
		if u, ok := unitMap[id]; ok {
			sumX += u.X
			sumY += u.Y
			n++
		}
	}
	if n == 0 {
		return false
	}
	sx, sy := float64(sumX/n), float64(sumY/n)
	bx, by := e.BuildingCentroid()
	dx := sx - float64(bx)
	dy := sy - float64(by)
	mw := float64(e.State.MapWidth)
	mh := float64(e.State.MapHeight)
	radius := math.Sqrt(mw*mw+mh*mh) * radiusPct
	return dx*dx+dy*dy > radius*radius
}

func (e RuleEnv) UnassignedIdleGround() []model.Unit {
	assigned := squadUnitIDSet(e.Memory)
	var out []model.Unit
	for _, u := range e.IdleGroundUnits() {
		if !assigned[u.ID] {
			out = append(out, u)
		}
	}
	return out
}

func (e RuleEnv) UnassignedIdleAir() []model.Unit {
	assigned := squadUnitIDSet(e.Memory)
	var out []model.Unit
	for _, u := range e.IdleCombatAircraft() {
		if !assigned[u.ID] {
			out = append(out, u)
		}
	}
	return out
}

func (e RuleEnv) UnassignedIdleNaval() []model.Unit {
	assigned := squadUnitIDSet(e.Memory)
	var out []model.Unit
	for _, u := range e.IdleNavalUnits() {
		if !assigned[u.ID] {
			out = append(out, u)
		}
	}
	return out
}
