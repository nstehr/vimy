package agent

import (
	"math"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
)

// Vimy has always counted what it loses and nothing of what it kills, so no
// engagement could be judged: a doctrine trading three for one and a doctrine
// being slaughtered produced the same numbers. Our own losses are read by
// diffing unit IDs between ticks, which is exact because our units are always
// visible. Enemies are not.
//
// An enemy that disappears has either died or walked into fog, and the two are
// indistinguishable from the outside. So a disappearance counts as a kill only
// when we were still looking at the place it disappeared from; everything else
// is counted separately as presumed. Reporting a presumed kill as a real one
// would tell the strategist it is winning fights it is losing, which is worse
// than the silence this replaces.
type killTracker struct {
	prevSeen map[int]enemySighting
	// Confident: last seen somewhere we still observe.
	Units     int
	Buildings int
	// Vanished out of sight. Some of these are kills; we cannot say which.
	PresumedUnits     int
	PresumedBuildings int
}

type enemySighting struct {
	Type string
	X, Y int
}

// observedFraction is how close one of our actors must be to a last-known
// position for us to claim we saw what happened there, as a fraction of the map
// diagonal. Deliberately tight: the cost of guessing wrong is a false kill.
const observedFraction = 0.06

func (k *killTracker) reset() {
	k.prevSeen = nil
	k.Units, k.Buildings = 0, 0
	k.PresumedUnits, k.PresumedBuildings = 0, 0
}

// observe diffs the visible enemy set against the previous tick.
func (k *killTracker) observe(gs model.GameState) {
	cur := make(map[int]enemySighting, len(gs.Enemies))
	for _, e := range gs.Enemies {
		cur[e.ID] = enemySighting{Type: e.Type, X: e.X, Y: e.Y}
	}

	if k.prevSeen != nil {
		threshSq := observedRadiusSq(gs)
		for id, seen := range k.prevSeen {
			if _, still := cur[id]; still {
				continue
			}
			building := rules.IsKnownBuildingType(seen.Type)
			if weWatched(gs, seen.X, seen.Y, threshSq) {
				if building {
					k.Buildings++
				} else {
					k.Units++
				}
				continue
			}
			if building {
				k.PresumedBuildings++
			} else {
				k.PresumedUnits++
			}
		}
	}
	k.prevSeen = cur
}

func observedRadiusSq(gs model.GameState) float64 {
	mw, mh := float64(gs.MapWidth), float64(gs.MapHeight)
	r := math.Sqrt(mw*mw+mh*mh) * observedFraction
	return r * r
}

// weWatched reports whether any actor of ours is close enough to the position
// that we would have seen what happened to whatever stood there. Buildings
// count: a refinery watches its own ground.
func weWatched(gs model.GameState, x, y int, threshSq float64) bool {
	for _, u := range gs.Units {
		dx, dy := float64(u.X-x), float64(u.Y-y)
		if dx*dx+dy*dy <= threshSq {
			return true
		}
	}
	for _, b := range gs.Buildings {
		dx, dy := float64(b.X-x), float64(b.Y-y)
		if dx*dx+dy*dy <= threshSq {
			return true
		}
	}
	return false
}
