package rules

import (
	"math"

	"github.com/nstehr/vimy/vimy-core/model"
)

// ThreatFieldFor computes the danger map from outside the rule loop, so
// telemetry can record what the router is actually reading. Same inputs the
// rules use; nil when there is no terrain to raster onto.
func ThreatFieldFor(memory map[string]any, terrain *model.TerrainGrid, state model.GameState) *model.ThreatField {
	if terrain == nil {
		return nil
	}
	return RuleEnv{Memory: memory, Terrain: terrain, State: state}.ThreatField()
}

func getEnemyStructures(memory map[string]any) map[int]EnemyDefenseIntel {
	if v, ok := memory["enemyStructures"].(map[int]EnemyDefenseIntel); ok {
		return v
	}
	return make(map[int]EnemyDefenseIntel)
}

// RememberedEnemyStructures is every enemy building whose position is known,
// defences included. Where enemyBuildingsSeen says what has been seen, this
// says where.
func RememberedEnemyStructures(memory map[string]any) map[int]EnemyDefenseIntel {
	return getEnemyStructures(memory)
}

// pruneStaleStructures drops a building once one of ours has stood where it
// should be and not seen it.
func pruneStaleStructures(env RuleEnv, structures map[int]EnemyDefenseIntel) {
	const clearRadiusSq = 10 * 10
	const minAge = 300
	for id, intel := range structures {
		if env.State.Tick-intel.Tick < minAge {
			continue
		}
		for _, u := range env.State.Units {
			dx, dy := u.X-intel.X, u.Y-intel.Y
			if dx*dx+dy*dy < clearRadiusSq {
				delete(structures, id)
				break
			}
		}
	}
}

// RememberedEnemyBases and RememberedEnemyDefenses expose what the AI actually
// reasons from, as opposed to what is on screen this instant.
//
// GameState.Enemies holds only CURRENTLY VISIBLE enemies, which over a whole
// game is a small minority of samples - 223 rows against 12071 of ours in game
// 175 - so a map drawn from it shows an empty enemy half while the squad is
// fighting. Targeting, the threat field and every approach decision run off
// this remembered intel instead, and it is the honest answer to "what does
// Vimy think is out there".
func RememberedEnemyBases(memory map[string]any) map[string]EnemyBaseIntel {
	return getEnemyBases(memory)
}

// RememberedEnemyDefenses is the defences the AI still believes are standing.
func RememberedEnemyDefenses(memory map[string]any) map[int]EnemyDefenseIntel {
	return getEnemyDefenses(memory)
}

func getEnemyDefenses(memory map[string]any) map[int]EnemyDefenseIntel {
	if v, ok := memory["enemyDefenses"].(map[int]EnemyDefenseIntel); ok {
		return v
	}
	return make(map[int]EnemyDefenseIntel)
}

// ThreatField rasterizes remembered defenses onto a field parallel to the
// terrain grid.
func (e RuleEnv) ThreatField() *model.ThreatField {
	if e.Terrain == nil {
		return nil
	}
	f := model.NewThreatField(e.Terrain)
	seen := 0
	for _, d := range getEnemyDefenses(e.Memory) {
		w, ok := defenseThreatWeight[d.Type]
		if !ok {
			w = 1.0
		}
		f.AddSource(e.Terrain, d.X, d.Y, w)
		seen++
	}
	e.addPresumedBaseThreat(f, seen)
	e.addRememberedDeaths(f)
	return f
}

// A death is evidence of a threat even when whatever caused it was never seen.
//
// This is the observation-only version of scouting: if six riflemen died at a
// spot, that spot is dangerous whether or not the tower that killed them was
// ever sighted. It needs no unit-type knowledge, it cannot be fooled by fog,
// and it is exactly what a human infers from watching their army evaporate at
// the same place twice.
//
// Weighted below a confirmed defence - a death says something killed you there,
// not what or whether it is still there - and expired, because a battle site
// from forty thousand ticks ago is not a standing threat.
const (
	deathThreatWeight   = 0.8
	deathMemoryTicks    = 20000
	maxRememberedDeaths = 64
)

// DeathSite is where one of ours was lost.
type DeathSite struct{ X, Y, Tick int }

func getDeathSites(memory map[string]any) []DeathSite {
	v, _ := memory["deathSites"].([]DeathSite)
	return v
}

// RecordDeathSite remembers where a unit was lost, dropping the oldest once the
// list is full so a long game cannot grow it without bound.
func RecordDeathSite(memory map[string]any, x, y, tick int) {
	sites := append(getDeathSites(memory), DeathSite{X: x, Y: y, Tick: tick})
	if len(sites) > maxRememberedDeaths {
		sites = sites[len(sites)-maxRememberedDeaths:]
	}
	memory["deathSites"] = sites
}

func (e RuleEnv) addRememberedDeaths(f *model.ThreatField) {
	for _, d := range getDeathSites(e.Memory) {
		if e.State.Tick-d.Tick > deathMemoryTicks {
			continue
		}
		f.AddSource(e.Terrain, d.X, d.Y, deathThreatWeight)
	}
}

// presumedDefenceWeight is what an unscouted base is assumed to be defended
// with, and presumedRingCells is how far out that assumption is painted.
//
// The field was built only from defences actually SIGHTED, which meant an
// unscouted base looked completely safe. Game 173 had observed one flame tower
// all game; the direct corridor scored 0.00 against an openEnoughThreshold of
// 1.0, so BestApproachAxis declared the front door open and switched the whole
// detour mechanism off while the squad walked into towers it had never seen.
//
// Every Red Alert base has defences. Assuming none is a stronger claim than
// assuming some, and it fails in the dangerous direction. So a known base
// carries a presumed ring until real sightings replace it - the weight is
// scaled down as defences are actually observed, reaching zero once enough are
// known that the real field speaks for itself.
const (
	presumedDefenceWeight = 1.2
	presumedRingCells     = 6
	presumedFadesAfter    = 4
)

// addPresumedBaseThreat paints a ring around each known enemy base, faded by
// how much real intel exists.
func (e RuleEnv) addPresumedBaseThreat(f *model.ThreatField, observed int) {
	if observed >= presumedFadesAfter {
		return
	}
	fade := 1.0 - float64(observed)/float64(presumedFadesAfter)
	w := presumedDefenceWeight * fade
	radius := presumedRingCells * maxInt(e.Terrain.CellW, e.Terrain.CellH)
	for _, base := range getEnemyBases(e.Memory) {
		for i := 0; i < 8; i++ {
			angle := float64(i) * math.Pi / 4
			x := base.X + int(float64(radius)*math.Cos(angle))
			y := base.Y + int(float64(radius)*math.Sin(angle))
			f.AddSource(e.Terrain, clampInt(x, 0, e.State.MapWidth-1), clampInt(y, 0, e.State.MapHeight-1), w)
		}
	}
}

// airDefenseThreatWeight covers only what actually shoots aircraft. Weighted
// above the ground equivalents: a strike package can't trade with a SAM cluster
// the way a ground push can trade with a pillbox.
var airDefenseThreatWeight = map[string]float64{
	AAGun:   2.0,
	SAMSite: 2.5,
}

// AirThreatField is the AA-only counterpart of ThreatField.
func (e RuleEnv) AirThreatField() *model.ThreatField {
	if e.Terrain == nil {
		return nil
	}
	f := model.NewThreatField(e.Terrain)
	for _, d := range getEnemyDefenses(e.Memory) {
		w, ok := airDefenseThreatWeight[d.Type]
		if !ok {
			continue
		}
		f.AddSource(e.Terrain, d.X, d.Y, w)
	}
	return f
}

// ApproachWaypoint is a staging point short of dest: the last low-threat zone
// on the weighted path, where a squad forms up before committing. False when a
// detour buys nothing — open approach, no intel, unreachable.
func (e RuleEnv) ApproachWaypoint(destX, destY int) (int, int, bool) {
	return e.approachWaypointWithField(destX, destY, e.ThreatField())
}

// AirApproachWaypoint mirrors ApproachWaypoint against AA defenses only.
func (e RuleEnv) AirApproachWaypoint(destX, destY int) (int, int, bool) {
	return e.approachWaypointWithField(destX, destY, e.AirThreatField())
}

// approachAxisRadiusCells places flank candidates far enough out to escape a
// defense cluster's painted radius — otherwise every candidate scores hot —
// while staying a small detour relative to the base-to-target distance.
const approachAxisRadiusCells = 8

// openEnoughThreshold is where a corridor counts as already open and the whole
// detour mechanism switches off. Roughly the signal of 1-2 distant defenses.
const openEnoughThreshold = 1.0

// BestApproachAxis returns the compass flank around dest whose corridor from
// base carries the least threat. Comparing eight candidates finds the open side
// of a lopsidedly defended base, which ApproachWaypoint's single weighted BFS
// path does not reliably do — hence this, not that, is the front door for squad
// routing. False when data is missing, the direct corridor is already open, or
// no flank is meaningfully cleaner.
func (e RuleEnv) BestApproachAxis(destX, destY int) (int, int, bool) {
	x, y, ok, why, direct, best := e.bestApproachAxisWithField(destX, destY, e.ThreatField())
	// Only the ground side is recorded. Four outcomes look identical from the
	// call site - it returns false three different ways - so "the squad walked
	// the front door" could not be told from "it checked and the front door was
	// the best option". Game 172 walked into flame towers repeatedly while
	// having observed exactly one of them, which is the no-intel case, but
	// nothing in the archive could say so.
	recordApproachChoice(e, why, direct, best)
	return x, y, ok
}

// BestAirApproachAxis is the AA-only counterpart of BestApproachAxis.
func (e RuleEnv) BestAirApproachAxis(destX, destY int) (int, int, bool) {
	x, y, ok, _, _, _ := e.bestApproachAxisWithField(destX, destY, e.AirThreatField())
	return x, y, ok
}

// bestApproachAxisWithField also reports WHY, so the three different ways of
// returning false can be told apart afterwards.
func (e RuleEnv) bestApproachAxisWithField(destX, destY int, field *model.ThreatField) (int, int, bool, string, float64, float64) {
	if e.Terrain == nil || e.Terrain.CellW <= 0 || e.Terrain.CellH <= 0 || field == nil {
		return 0, 0, false, ApproachNoData, 0, 0
	}
	centX, centY := e.BuildingCentroid()

	directScore := corridorThreatSum(field, e.Terrain, centX, centY, destX, destY)
	if directScore < openEnoughThreshold {
		// Below the threshold the corridor counts as open and the whole
		// mechanism switches off. With one defence observed it is always below.
		return 0, 0, false, ApproachOpen, directScore, 0
	}

	radiusMap := approachAxisRadiusCells * maxInt(e.Terrain.CellW, e.Terrain.CellH)

	bestScore := math.Inf(1)
	var bestX, bestY int
	for i := 0; i < 8; i++ {
		angle := float64(i) * math.Pi / 4
		wx := destX + int(float64(radiusMap)*math.Cos(angle))
		wy := destY + int(float64(radiusMap)*math.Sin(angle))
		wx = clampInt(wx, 0, e.State.MapWidth-1)
		wy = clampInt(wy, 0, e.State.MapHeight-1)
		score := corridorThreatSum(field, e.Terrain, centX, centY, wx, wy)
		if score < bestScore {
			bestScore = score
			bestX = wx
			bestY = wy
		}
	}

	// A margin, not a strict win: near-tied candidates would re-route constantly.
	if bestScore >= directScore*0.8 {
		return 0, 0, false, ApproachNoBetterFlank, directScore, bestScore
	}
	return bestX, bestY, true, ApproachDetour, directScore, bestScore
}

// corridorThreatSum sums threat field cells in the bounding box between two
// map positions.
func corridorThreatSum(field *model.ThreatField, t *model.TerrainGrid, ax, ay, bx, by int) float64 {
	if t == nil || t.CellW <= 0 || t.CellH <= 0 {
		return 0
	}
	aCol, aRow := ax/t.CellW, ay/t.CellH
	bCol, bRow := bx/t.CellW, by/t.CellH
	minCol, maxCol := aCol, bCol
	if minCol > maxCol {
		minCol, maxCol = maxCol, minCol
	}
	minRow, maxRow := aRow, bRow
	if minRow > maxRow {
		minRow, maxRow = maxRow, minRow
	}
	sum := 0.0
	for r := minRow; r <= maxRow; r++ {
		for c := minCol; c <= maxCol; c++ {
			sum += field.At(c, r)
		}
	}
	return sum
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (e RuleEnv) approachWaypointWithField(destX, destY int, field *model.ThreatField) (int, int, bool) {
	if e.Terrain == nil || e.Terrain.CellW <= 0 || e.Terrain.CellH <= 0 {
		return 0, 0, false
	}
	centX, centY := e.BuildingCentroid()
	from := [2]int{centX / e.Terrain.CellW, centY / e.Terrain.CellH}
	to := [2]int{destX / e.Terrain.CellW, destY / e.Terrain.CellH}
	if field == nil {
		return 0, 0, false
	}

	const penalty = 5.0
	const bboxHeatThreshold = 1.0 // cumulative threat in the corridor between base and goal
	const hotThreshold = 0.4      // per-zone threat that marks a zone as contested

	// A bounding-box scan rather than the heat along the direct path: which
	// shortest path BFS returns is a tie-break artifact, and defenses between
	// us and the target matter whichever one it picks.
	minCol, maxCol := from[0], to[0]
	if minCol > maxCol {
		minCol, maxCol = maxCol, minCol
	}
	minRow, maxRow := from[1], to[1]
	if minRow > maxRow {
		minRow, maxRow = maxRow, minRow
	}
	bboxHeat := 0.0
	for row := minRow; row <= maxRow; row++ {
		for col := minCol; col <= maxCol; col++ {
			bboxHeat += field.At(col, row)
		}
	}
	if bboxHeat < bboxHeatThreshold {
		return 0, 0, false
	}

	// Stage at the last cool zone before the path turns hot; failing that, just
	// short of the goal on whatever approach the weighted BFS chose.
	weighted := model.ApproachPath(e.Terrain, field, from, to, penalty)
	if len(weighted) < 2 {
		return 0, 0, false
	}

	var wp [2]int
	found := false
	for i, p := range weighted {
		if i == 0 {
			continue
		}
		if field.At(p[0], p[1]) >= hotThreshold {
			wp = weighted[i-1]
			found = true
			break
		}
	}
	if !found {
		wp = weighted[len(weighted)-2]
		found = true
	}
	if wp == from {
		return 0, 0, false
	}
	x, y := e.Terrain.ZoneCenter(wp[0], wp[1])
	return x, y, true
}
