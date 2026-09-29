package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// A captured oil derrick is a building we own and is nowhere near the base.
//
// Game 193 held three, at 28, 45 and 76 cells from the construction yard, and
// the threshold is 20 percent of the map diagonal — about 26 cells on a 91x91
// map. 52 percent of defend-base's 606 firings that game had the nearest enemy
// beside a DERRICK and nothing within 26 cells of the real base, which recalled
// the army from the enemy's doorstep to defend an oil well.
func TestBaseUnderAttackIgnoresCapturedTechStructures(t *testing.T) {
	// Construction yard at (12,65); a captured derrick out at (58,4).
	env := func(enemyX, enemyY int) RuleEnv {
		return RuleEnv{State: model.GameState{
			MapWidth: 91, MapHeight: 91,
			Buildings: []model.Building{
				{Type: "fact", X: 12, Y: 65},
				{Type: "oilb", X: 58, Y: 4},
			},
			Enemies: []model.Enemy{{Type: "3tnk", X: enemyX, Y: enemyY}},
		}}
	}

	// Beside the derrick, 76 cells from the base: not an attack on the base.
	if env(58, 6).BaseUnderAttack() {
		t.Error("an enemy beside a captured derrick must not count as the base being attacked")
	}
	// Beside the construction yard: it certainly is.
	if !env(14, 67).BaseUnderAttack() {
		t.Error("an enemy at the construction yard must count")
	}
	// The derrick must not be the only thing keeping the base alive either.
	only := RuleEnv{State: model.GameState{
		MapWidth: 91, MapHeight: 91,
		Buildings: []model.Building{{Type: "oilb", X: 58, Y: 4}},
		Enemies:   []model.Enemy{{Type: "3tnk", X: 58, Y: 6}},
	}}
	if only.BaseUnderAttack() {
		t.Error("with nothing but a derrick owned, there is no base to be under attack")
	}
}
