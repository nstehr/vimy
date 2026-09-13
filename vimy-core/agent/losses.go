package agent

import (
	"math"

	"github.com/nstehr/vimy/vimy-core/model"
)

// Where a unit was standing when we lost it.
//
// Armour peak has been 5, 6, 2, 4 and 1 across games whose vehicle production
// ranged from 6 orders to 1167 — a two-hundredfold swing that moved the
// standing army not at all. Game 112 built roughly 48 vehicles and held 6. They
// die about as fast as they appear and the counters cannot say where, so every
// explanation offered for it so far has been a story.
//
// Forward and at home imply opposite fixes. Forward means armour is being fed
// into flame towers in mixed waves and the answer is composition or siege.
// At home means base defence is eating the army meant for attacking, and no
// change to how squads are built would help.
type lossTracker struct {
	prevSeen map[int]unitSighting
	Forward  map[string]int
	NearBase map[string]int
}

type unitSighting struct {
	Domain string
	X, Y   int
}

// homeFraction is how far from the base centroid still counts as home, as a
// fraction of the map diagonal. The same 20% NearBaseGroundUnits uses to decide
// a unit is available for base defence, so the two agree about where home is.
const homeFraction = 0.20

func (l *lossTracker) reset() {
	l.prevSeen = nil
	l.Forward = nil
	l.NearBase = nil
}

func (l *lossTracker) observe(gs model.GameState) {
	cur := make(map[int]unitSighting, len(gs.Units))
	for _, u := range gs.Units {
		if d := unitDomain(u); d != "" {
			cur[u.ID] = unitSighting{Domain: d, X: u.X, Y: u.Y}
		}
	}

	if l.prevSeen != nil && len(gs.Buildings) > 0 {
		if l.Forward == nil {
			l.Forward = map[string]int{}
			l.NearBase = map[string]int{}
		}
		cx, cy := buildingCentroid(gs)
		mw, mh := float64(gs.MapWidth), float64(gs.MapHeight)
		threshold := math.Sqrt(mw*mw+mh*mh) * homeFraction
		threshSq := threshold * threshold

		for id, seen := range l.prevSeen {
			if _, alive := cur[id]; alive {
				continue
			}
			dx, dy := float64(seen.X-cx), float64(seen.Y-cy)
			if dx*dx+dy*dy <= threshSq {
				l.NearBase[seen.Domain]++
			} else {
				l.Forward[seen.Domain]++
			}
		}
	}
	l.prevSeen = cur
}

func buildingCentroid(gs model.GameState) (int, int) {
	sx, sy := 0, 0
	for _, b := range gs.Buildings {
		sx += b.X
		sy += b.Y
	}
	n := len(gs.Buildings)
	return sx / n, sy / n
}
