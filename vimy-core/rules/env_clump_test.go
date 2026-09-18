package rules

import (
	"fmt"
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
