package agent

import "github.com/nstehr/vimy/vimy-core/model"

// What the two armies are worth, sampled as the game runs.
//
// The army panel in game 119 showed 31 units of Vimy's against 30 of the
// opponent's — and 5200 credits against 14700. Riflemen against heavy tanks,
// V2 rockets and shock troopers. Nothing in the sidecar could say that: the
// engine reports our own army value and nothing of theirs, and the end-of-game
// figure is taken when our army is already dead, so it reads 0 or 100 and
// describes nothing.
//
// Ours comes from the engine. Theirs is the sum of what we can see, which
// undercounts whatever sits in fog — so the gap this measures is a floor on the
// real one.
type armyValueTracker struct {
	OursPeak  int
	TheirsPeak int
	// Summed at each sample so the averages describe the game rather than its
	// last moment.
	oursTotal   int
	theirsTotal int
	samples     int
}

func (a *armyValueTracker) reset() { *a = armyValueTracker{} }

func (a *armyValueTracker) observe(gs model.GameState) {
	ours := gs.Player.ArmyValue
	theirs := 0
	for _, e := range gs.Enemies {
		theirs += e.Cost
	}
	if ours > a.OursPeak {
		a.OursPeak = ours
	}
	if theirs > a.TheirsPeak {
		a.TheirsPeak = theirs
	}
	a.oursTotal += ours
	a.theirsTotal += theirs
	a.samples++
}

// Means over the game, zero before anything has been seen.
func (a *armyValueTracker) Means() (ours, theirs int) {
	if a.samples == 0 {
		return 0, 0
	}
	return a.oursTotal / a.samples, a.theirsTotal / a.samples
}
