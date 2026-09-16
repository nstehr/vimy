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
	OursPeak   int
	TheirsPeak int
	// Summed at each sample so the averages describe the game rather than its
	// last moment.
	oursTotal int
	samples   int
	// Theirs is averaged only over samples where any of it was visible.
	// Including the blind ticks made an army of 8500 average out to 460 and
	// read as small, when it was mostly just out of sight.
	theirsTotal   int
	theirsSamples int
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
	a.samples++
	if theirs > 0 {
		a.theirsTotal += theirs
		a.theirsSamples++
	}
}

// Means over the game: ours across every sample, theirs across the samples
// where we could see any of it. The two are not averaged over the same
// denominator on purpose — theirs answers "how big is what we run into", not
// "how much of the map is empty".
func (a *armyValueTracker) Means() (ours, theirs int) {
	if a.samples > 0 {
		ours = a.oursTotal / a.samples
	}
	if a.theirsSamples > 0 {
		theirs = a.theirsTotal / a.theirsSamples
	}
	return ours, theirs
}
