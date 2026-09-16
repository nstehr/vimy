package agent

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// The army panel in game 119 showed 31 units against 30 — and 5200 credits
// against 14700. Counting heads said the armies were even; pricing them said
// one was worth nearly three of the other.
func TestArmyValueTracksBothSidesOverTheGame(t *testing.T) {
	var a armyValueTracker

	// Early: a few riflemen against a couple of heavy tanks.
	a.observe(model.GameState{
		Player: model.Player{ArmyValue: 300},
		Enemies: []model.Enemy{
			{ID: 1, Type: "3tnk", Cost: 1150},
			{ID: 2, Type: "3tnk", Cost: 1150},
		},
	})
	// Later: our peak, and more of theirs in view.
	a.observe(model.GameState{
		Player: model.Player{ArmyValue: 5200},
		Enemies: []model.Enemy{
			{ID: 1, Type: "3tnk", Cost: 1150},
			{ID: 2, Type: "3tnk", Cost: 1150},
			{ID: 3, Type: "v2rl", Cost: 900},
			{ID: 4, Type: "4tnk", Cost: 1700},
		},
	})
	// Late: our army is gone, which is exactly why the end-of-game figure
	// describes nothing.
	a.observe(model.GameState{Player: model.Player{ArmyValue: 0}})

	if a.OursPeak != 5200 {
		t.Errorf("our peak = %d, want 5200", a.OursPeak)
	}
	if a.TheirsPeak != 4900 {
		t.Errorf("their peak = %d, want 4900", a.TheirsPeak)
	}
	ours, theirs := a.Means()
	if ours != (300+5200+0)/3 {
		t.Errorf("our mean = %d, want %d", ours, (300+5200)/3)
	}
	// Theirs averages only the samples where any of it was visible. Counting
	// the blind third would report an army of 4900 as 2400 and read as small
	// when it was merely out of sight.
	if want := (2300 + 4900) / 2; theirs != want {
		t.Errorf("their mean = %d, want %d — blind samples must not dilute it", theirs, want)
	}

	// A reset must not carry one game's armies into the next.
	a.reset()
	if o, tt := a.Means(); a.OursPeak != 0 || a.TheirsPeak != 0 || o != 0 || tt != 0 {
		t.Error("reset left values behind")
	}
}

// An enemy never seen at all is not an enemy of size zero; it is an unknown.
func TestArmyValueReportsNoEnemyMeanWhenNoneWasEverSeen(t *testing.T) {
	var a armyValueTracker
	for range 5 {
		a.observe(model.GameState{Player: model.Player{ArmyValue: 900}})
	}
	ours, theirs := a.Means()
	if ours != 900 {
		t.Errorf("our mean = %d, want 900", ours)
	}
	if theirs != 0 || a.TheirsPeak != 0 {
		t.Errorf("reported an enemy army (%d/%d) that was never observed", theirs, a.TheirsPeak)
	}
}
