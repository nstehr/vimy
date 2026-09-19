package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func criticalEnv(hp, maxHP, enemyX int) RuleEnv {
	return RuleEnv{
		Memory: map[string]any{},
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 50, Y: 50, HP: hp, MaxHP: maxHP},
			},
			Enemies: []model.Enemy{
				{ID: 9, Type: "2tnk", X: enemyX, Y: 50, HP: 400, MaxHP: 400},
			},
		},
	}
}

// The gate lets defend-critical-building conscript every nearby unit, squad
// members included. It used to fire on proximity alone: any enemy within 10
// cells of the construction yard pulled the whole army home. Game 137 acted on
// it 536 times in 48460 ticks and it replaced emergency-base-defense as the
// thing dragging the assault back.
func TestCriticalDefenceNeedsDamageAndProximity(t *testing.T) {
	cases := []struct {
		name            string
		hp, max, enemyX int
		want            bool
	}{
		{"hit, and they are still on it", 900, 1000, 55, true},
		{"undamaged, enemy merely driving past", 1000, 1000, 55, false},
		{"scarred, but nobody near it", 900, 1000, 120, false},
		{"undamaged and nobody near", 1000, 1000, 120, false},
		{"badly hit with the attacker adjacent", 100, 1000, 51, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := criticalEnv(c.hp, c.max, c.enemyX).CriticalBuildingUnderAttack()
			if got != c.want {
				t.Errorf("under attack = %v, want %v", got, c.want)
			}
		})
	}
}

// Only core infrastructure counts. A damaged pillbox with an enemy on it is a
// normal fight, not a reason to abandon an offensive.
func TestCriticalDefenceIgnoresNonCriticalBuildings(t *testing.T) {
	env := criticalEnv(900, 1000, 55)
	env.State.Buildings[0].Type = "pbox"
	if env.CriticalBuildingUnderAttack() {
		t.Error("a damaged pillbox must not trigger the army-wide override")
	}
}

// Husks linger in State.Enemies after death and would otherwise hold the whole
// defence on an already-dead attacker beside a scarred building.
func TestCriticalDefenceIgnoresHusks(t *testing.T) {
	env := criticalEnv(900, 1000, 55)
	env.State.Enemies[0].Type = "2tnk.husk"
	if env.CriticalBuildingUnderAttack() {
		t.Error("a husk must not count as an attacker")
	}
}
