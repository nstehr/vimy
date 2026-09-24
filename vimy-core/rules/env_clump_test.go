package rules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// clumpEnv puts n squad members on a line: all at the centroid except the
// last `stragglers`, parked far enough out to fail any sane radius.
func clumpEnv(n, stragglers int) RuleEnv {
	sq := &Squad{}
	var units []model.Unit
	for i := range n {
		id := i + 1
		sq.UnitIDs = append(sq.UnitIDs, id)
		x := 50
		if i >= n-stragglers {
			x = 100 // far away
		}
		units = append(units, model.Unit{ID: id, Type: "e1", X: x, Y: 50})
	}
	return RuleEnv{
		Memory: map[string]any{"squads": map[string]*Squad{"s": sq}},
		State:  model.GameState{Units: units},
	}
}

// The check is documented as 80%, and the obvious integer form rounded UP: a
// squad of four needed 4 of 4, and three or two needed all of them too. Vimy's
// squads averaged 3.5 members when told to re-gather, so the gate it actually
// applied was "every unit within 8 cells". Games 132 and 133 put 110 of 165
// failed strikes on this check and reached `strike` zero times.
//
// The second half of the same bug was the centroid: a mean is dragged by the
// straggler, so three units at 50, 50 and 100 have a centre of 66 and NOBODY
// is within 8 cells of it. Allowing a straggler means nothing while one
// straggler disqualifies everybody, so the centre is now a median.
func TestClumping(t *testing.T) {
	cases := []struct {
		members, stragglers int
		want                bool
		why                 string
	}{
		// The fix: a small squad may now lose one unit and still attack.
		{3, 1, true, "three with one lagging used to need all three"},
		{4, 1, true, "four with one lagging used to need all four"},
		// A pair has no minority; one unit walking in alone is the piecemeal
		// death this check exists to prevent.
		{2, 1, false, "a pair must arrive together"},
		{2, 0, true, "a pair standing together is clumped"},
		// Unchanged above four: 80% was already expressible there.
		{5, 1, true, "five tolerates one"},
		{6, 1, true, "six tolerates one"},
		{10, 2, true, "ten with two out is exactly 80%"},
		// The slack is one unit's worth, not a free-for-all.
		{4, 2, false, "four with two out is half"},
		{5, 2, false, "five with two out is under 80%"},
		{6, 2, false, "six with two out is under 80%"},
		{10, 3, false, "ten with three out is under 80%"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d_members_%d_out", c.members, c.stragglers), func(t *testing.T) {
			if got := clumpEnv(c.members, c.stragglers).SquadClumped("s", 8); got != c.want {
				t.Errorf("clumped = %v, want %v: %s", got, c.want, c.why)
			}
		})
	}
}

// The centroid must sit with the cluster, not be pulled into empty ground
// between the cluster and an outlier.
func TestCentroidIgnoresTheOutlier(t *testing.T) {
	units := []model.Unit{
		{ID: 1, X: 50, Y: 50}, {ID: 2, X: 50, Y: 50}, {ID: 3, X: 100, Y: 100},
	}
	x, y := medianXY(units)
	if x != 50 || y != 50 {
		t.Errorf("centre = (%d,%d), want (50,50): the mean would be (66,66), with nobody near it", x, y)
	}
}

// The gate's numerator, exposed.
//
// Game 154 blocked 80 strikes on dispersion and recorded members and the
// furthest straggler — a maximum, where the gate reads a percentile. Those two
// numbers cannot distinguish "25 of 31 packed tight and 6 trailing", which
// passes, from "31 evenly strung out", which cannot pass at any squad size the
// 8-cell radius was tuned for. near is the figure that separates them.
func TestSquadClumpReportsWhatTheGateCounted(t *testing.T) {
	// Six units: four together, two a long way out. 80% of six is five, so
	// four near is a fail — and the max spread alone would not have said so.
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, X: 100, Y: 100},
				{ID: 2, X: 101, Y: 100},
				{ID: 3, X: 100, Y: 101},
				{ID: 4, X: 102, Y: 102},
				{ID: 5, X: 140, Y: 100},
				{ID: 6, X: 160, Y: 100},
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", Domain: "ground", UnitIDs: []int{1, 2, 3, 4, 5, 6}},
			},
		},
	}
	members, near, need := env.SquadClump("ground-attack", 8)
	if members != 6 {
		t.Errorf("members = %d, want 6", members)
	}
	if near != 4 {
		t.Errorf("near = %d, want 4: two units are 40 and 60 cells out", near)
	}
	if need != 5 {
		t.Errorf("need = %d, want 5 (ceil(0.8*6))", need)
	}
	// And the wrapper must still be exactly near >= need.
	if env.SquadClumped("ground-attack", 8) {
		t.Error("SquadClumped disagrees with its own arithmetic")
	}
}

// Every trivially-clumped shortcut must keep answering true through the split:
// no squad map, no squad, and a single surviving unit.
func TestSquadClumpTrivialCasesStayClumped(t *testing.T) {
	cases := map[string]RuleEnv{
		"no squad map": {State: model.GameState{}, Memory: map[string]any{}},
		"no such squad": {
			State:  model.GameState{},
			Memory: map[string]any{"squads": map[string]*Squad{}},
		},
		"one survivor": {
			State: model.GameState{Units: []model.Unit{{ID: 1, X: 5, Y: 5}}},
			Memory: map[string]any{"squads": map[string]*Squad{
				"ground-attack": {Name: "ground-attack", UnitIDs: []int{1, 2, 3}},
			}},
		},
	}
	for name, env := range cases {
		if !env.SquadClumped("ground-attack", 8) {
			t.Errorf("%s: want trivially clumped", name)
		}
		_, near, need := env.SquadClump("ground-attack", 8)
		if near < need {
			t.Errorf("%s: near %d < need %d", name, near, need)
		}
	}
}

// An unscouted base is not a safe base.
//
// ThreatField was built only from defences actually SIGHTED, so a base nobody
// had scouted scored zero threat everywhere. Game 173 had observed one flame
// tower; the direct corridor scored 0.00 against an openEnoughThreshold of 1.0,
// so BestApproachAxis declared the front door open and disabled the detour
// while the squad walked into towers it had never seen. Assuming no defences is
// a stronger claim than assuming some, and it fails in the dangerous direction.
func TestUnscoutedBaseCarriesPresumedThreat(t *testing.T) {
	terrain := &model.TerrainGrid{Cols: 32, Rows: 32, CellW: 4, CellH: 4}
	env := RuleEnv{
		Terrain: terrain,
		State:   model.GameState{Tick: 5000, MapWidth: 128, MapHeight: 128},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"red": {Owner: "red", X: 100, Y: 100, Tick: 1, FromBuildings: true},
			},
		},
	}
	f := env.ThreatField()
	if f == nil {
		t.Fatal("no threat field")
	}
	var total float64
	for c := 0; c < f.Cols; c++ {
		for r := 0; r < f.Rows; r++ {
			total += f.At(c, r)
		}
	}
	if total <= 0 {
		t.Fatal("a known but unscouted base painted zero threat: the detour will switch itself off")
	}

	// And the presumption fades once real defences are known, so it cannot
	// drown out the actual field.
	withIntel := env
	defs := map[int]EnemyDefenseIntel{}
	for i := 0; i < presumedFadesAfter; i++ {
		defs[i] = EnemyDefenseIntel{Type: Pillbox, X: 100 + i, Y: 100}
	}
	withIntel.Memory = map[string]any{
		"enemyBases":    env.Memory["enemyBases"],
		"enemyDefenses": defs,
	}
	var presumed float64
	fi := withIntel.ThreatField()
	for c := 0; c < fi.Cols; c++ {
		for r := 0; r < fi.Rows; r++ {
			presumed += fi.At(c, r)
		}
	}
	// AddSource paints 1 + 4*(1/2) + 4*(1/3) = 4.333 per unit of weight, so four
	// real pillboxes at 1.0 come to 17.33. Anything above that is the
	// presumption still contributing when it should have faded to nothing.
	const fourRealPillboxes = 4 * 4.3333
	if presumed > fourRealPillboxes+0.01 {
		t.Errorf("presumed threat did not fade with intel: total %.2f, real defences alone are %.2f", presumed, fourRealPillboxes)
	}
}

// A death is evidence of a threat even when nothing was ever sighted.
func TestDeathsAreRememberedAsThreat(t *testing.T) {
	terrain := &model.TerrainGrid{Cols: 32, Rows: 32, CellW: 4, CellH: 4}
	mem := map[string]any{}
	RecordDeathSite(mem, 60, 60, 1000)
	env := RuleEnv{Terrain: terrain, State: model.GameState{Tick: 1100, MapWidth: 128, MapHeight: 128}, Memory: mem}
	if got := env.ThreatField().At(60/4, 60/4); got <= 0 {
		t.Errorf("a death site painted no threat: %v", got)
	}

	// And it expires, because an old battle is not a standing defence.
	stale := RuleEnv{Terrain: terrain, State: model.GameState{Tick: 1000 + deathMemoryTicks + 1, MapWidth: 128, MapHeight: 128}, Memory: mem}
	if got := stale.ThreatField().At(60/4, 60/4); got != 0 {
		t.Errorf("a death from %d ticks ago still counts: %v", deathMemoryTicks+1, got)
	}
}

// Enemy building POSITIONS must be remembered, not just their types.
//
// The sighting code recorded buildings as enemyBuildingsSeen, a map of type to
// count, and discarded x and y. So Vimy could know it had seen a refinery and
// never where one was. Targeting reads currently-visible enemies, so a building
// that went back into fog stopped existing - which is why the base assault
// spirals outward from a remembered centroid, and why blind-at-base fired 37
// times in game 173 with the squad standing on the enemy base.
func TestEnemyBuildingPositionsAreRemembered(t *testing.T) {
	mem := map[string]any{}
	env := RuleEnv{
		State: model.GameState{
			Tick: 1000,
			Enemies: []model.Enemy{
				{ID: 10, Type: "proc", X: 80, Y: 80, HP: 900, MaxHP: 900},
				{ID: 11, Type: "pbox", X: 78, Y: 82, HP: 400, MaxHP: 400},
				{ID: 12, Type: "3tnk", X: 70, Y: 70, HP: 400, MaxHP: 400},
			},
		},
		Memory: mem,
	}
	updateDefenseIntel(env)

	got := RememberedEnemyStructures(mem)
	if _, ok := got[10]; !ok {
		t.Error("a refinery was seen and its position forgotten")
	}
	if _, ok := got[11]; !ok {
		t.Error("a pillbox is a building too and must be here")
	}
	if _, ok := got[12]; ok {
		t.Error("a tank was recorded as a structure")
	}
	if got[10].X != 80 || got[10].Y != 80 {
		t.Errorf("refinery remembered at (%d,%d), want (80,80)", got[10].X, got[10].Y)
	}

	// And it is forgotten once one of ours stands there and sees nothing -
	// the same evidence standard the defences use.
	later := RuleEnv{
		State: model.GameState{
			Tick:  1000 + 400,
			Units: []model.Unit{{ID: 1, Type: "2tnk", X: 80, Y: 80}},
		},
		Memory: mem,
	}
	updateDefenseIntel(later)
	if _, ok := RememberedEnemyStructures(mem)[10]; ok {
		t.Error("a razed refinery is still remembered after standing on its site")
	}
}

// GameState.Capturables is not a list of neutral objectives.
//
// In one game it carried the enemy's construction yard, refineries, power, a
// SAM site and a flame tower, plus their APCs, flak trucks, heavy tanks,
// harvesters and a tank husk - alongside four oil derricks. Everything in it is
// capturable by something; only a few are NEUTRAL, and only those are what
// capture_priority and the capture rules are written about. Treating the whole
// list as neutral put yellow objective markers on enemy armour.
func TestOnlyRealTechStructuresAreNeutral(t *testing.T) {
	neutral := []string{"oilb", "fcom", "miss", "bio", "hosp"}
	for _, ty := range neutral {
		if !IsNeutralTechStructure(ty) {
			t.Errorf("%s is a neutral tech structure and was not recognised", ty)
		}
	}
	// Everything actually observed in that list which is not one.
	for _, ty := range []string{"apc", "ftrk", "3tnk", "harv", "3tnk.husk", "proc", "powr", "ftur", "fact", "sam", "badr"} {
		if IsNeutralTechStructure(ty) {
			t.Errorf("%s was treated as a neutral objective", ty)
		}
	}
	// Faction suffixes must not defeat it.
	if !IsNeutralTechStructure("oilb.ukraine") {
		t.Error("a faction-suffixed derrick was not recognised")
	}
}

// An engineer cannot capture a tank.
//
// GameState.Capturables carries whatever something could take: in one game the
// enemy's APCs, flak trucks, heavy tanks, harvesters and a husk, alongside
// their base and four derricks. Everything unrecognised scored
// capturableValueDefault, so walking an engineer at an enemy heavy tank was a
// legal choice and sometimes the highest-scoring one.
func TestCaptureTargetsMustBeStructures(t *testing.T) {
	at := func(ty string, x, y int) model.Enemy {
		return model.Enemy{ID: len(ty) + x, Type: ty, X: x, Y: y, HP: 500, MaxHP: 500}
	}
	env := func(caps ...model.Enemy) RuleEnv {
		return RuleEnv{
			State: model.GameState{
				MapWidth: 128, MapHeight: 128,
				Buildings:   []model.Building{{ID: 1, X: 10, Y: 10}},
				Capturables: caps,
			},
			Memory: map[string]any{},
		}
	}

	// Only vehicles on offer: nothing is a valid capture.
	if got := env(at("3tnk", 20, 20), at("apc", 22, 20), at("harv", 24, 20), at("3tnk.husk", 26, 20)).BestCapturable(); got != nil {
		t.Errorf("chose %s: an engineer cannot capture a vehicle", got.Type)
	}

	// An enemy structure is a legitimate target - taking their refinery is a
	// real tactic - and must still be chosen over a nearer tank.
	if got := env(at("3tnk", 12, 12), at("proc", 40, 40)).BestCapturable(); got == nil || got.Type != "proc" {
		t.Errorf("got %v, want the refinery even though the tank is nearer", got)
	}

	// A faction-suffixed derrick must rank on its real value, not the default.
	// Placed FURTHER away than an enemy power plant, so only its value can win.
	got := env(at("powr", 20, 20), at("oilb.ukraine", 60, 60)).BestCapturable()
	if got == nil || !strings.HasPrefix(got.Type, "oilb") {
		t.Errorf("got %v, want the derrick: value 10 over a power plant's default 2", got)
	}
}
