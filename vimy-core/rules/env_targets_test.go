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

// Everything that asks "where is home" must ignore captured tech structures.
//
// BuildingCentroid is the one that surfaced it: recall-stray-units walks units
// to that point and fired 134 times against squad-attack-known-base's 204,
// pulling infantry away from the enemy base it was attacking. The same
// assumption was in BaseUnderAttack, NearBaseGroundUnits, defenceHint, the
// harvester flee fallback and every "our position" reference point.
func TestHomeIsTheBaseNotTheDerricks(t *testing.T) {
	// Base at (12,65); three derricks scattered as in game 193.
	e := RuleEnv{State: model.GameState{
		MapWidth: 91, MapHeight: 91,
		Buildings: []model.Building{
			{Type: "fact", X: 12, Y: 65},
			{Type: "proc", X: 14, Y: 63},
			{Type: "oilb", X: 31, Y: 85},
			{Type: "oilb", X: 32, Y: 25},
			{Type: "oilb", X: 58, Y: 4},
		},
	}}

	cx, cy := e.BuildingCentroid()
	if cx != 13 || cy != 64 {
		t.Errorf("BuildingCentroid = (%d,%d), want the base at (13,64) — the derricks dragged it", cx, cy)
	}
	ax, ay, ok := e.baseAnchor()
	if !ok || ax != 12 || ay != 65 {
		t.Errorf("baseAnchor = (%d,%d) ok=%v, want the construction yard", ax, ay, ok)
	}
	if got := len(e.baseBuildings()); got != 2 {
		t.Errorf("baseBuildings = %d, want 2 — three derricks excluded", got)
	}

	// Owning nothing but derricks: fall back to them rather than the map
	// origin, because a derrick is still a better reference than (0,0).
	only := RuleEnv{State: model.GameState{
		MapWidth: 91, MapHeight: 91,
		Buildings: []model.Building{{Type: "oilb", X: 58, Y: 4}},
	}}
	if x, y := only.BuildingCentroid(); x != 58 || y != 4 {
		t.Errorf("with only a derrick owned = (%d,%d), want (58,4) not the origin", x, y)
	}
}

// Enemy siege outranks the screen in front of it.
//
// Every mobile type used to fall through to groundTargetValueDefault of 1.0, so
// a v2 launcher scored what a rifleman scored, and the distance decay then
// handed it to whichever was closer. A squad being shelled would shoot the
// infantry screen and leave the artillery firing. Watching the games is what
// surfaced this; the table is what confirmed it.
func TestEnemySiegeOutscoresTheScreenInFrontOfIt(t *testing.T) {
	env := RuleEnv{State: model.GameState{
		Tick: 20000, MapWidth: 128, MapHeight: 128,
		Buildings: []model.Building{{ID: 1, Type: "fact", X: 10, Y: 10, HP: 1000, MaxHP: 1000}},
		Enemies: []model.Enemy{
			// A rifleman right on top of the squad.
			{ID: 1, Type: "e1", X: 45, Y: 40, HP: 50, MaxHP: 50},
			// A v2 three times further away, behind the screen.
			{ID: 2, Type: "v2rl", X: 60, Y: 40, HP: 150, MaxHP: 150},
		},
	}}

	got := env.BestGroundTargetFrom(40, 40)
	if got == nil {
		t.Fatal("no target chosen")
	}
	if got.Type != "v2rl" {
		t.Errorf("chose %q at range, want v2rl: the thing shelling the squad has to outscore "+
			"the screen standing in front of it", got.Type)
	}
}

// Allied artillery is scored the same way, so the rule is about siege and not
// about the Soviet roster.
func TestEnemyArtilleryIsScoredAsSiegeToo(t *testing.T) {
	env := RuleEnv{State: model.GameState{
		Tick: 20000, MapWidth: 128, MapHeight: 128,
		Buildings: []model.Building{{ID: 1, Type: "fact", X: 10, Y: 10, HP: 1000, MaxHP: 1000}},
		Enemies: []model.Enemy{
			{ID: 1, Type: "e3", X: 45, Y: 40, HP: 45, MaxHP: 45},
			{ID: 2, Type: "arty", X: 58, Y: 40, HP: 75, MaxHP: 75},
		},
	}}
	if got := env.BestGroundTargetFrom(40, 40); got == nil || got.Type != "arty" {
		t.Errorf("chose %v, want arty", got)
	}
}

// A tank does NOT get siege treatment: armour can be fought on equal terms, and
// valuing everything mobile would restore "chase the nearest thing" with extra
// steps. The near rifleman should still win against a distant tank.
func TestTanksAreNotScoredAsSiege(t *testing.T) {
	env := RuleEnv{State: model.GameState{
		Tick: 20000, MapWidth: 128, MapHeight: 128,
		Buildings: []model.Building{{ID: 1, Type: "fact", X: 10, Y: 10, HP: 1000, MaxHP: 1000}},
		Enemies: []model.Enemy{
			{ID: 1, Type: "e1", X: 42, Y: 40, HP: 50, MaxHP: 50},
			{ID: 2, Type: "3tnk", X: 70, Y: 40, HP: 400, MaxHP: 400},
		},
	}}
	if got := env.BestGroundTargetFrom(40, 40); got == nil || got.Type != "e1" {
		t.Errorf("chose %v, want the near e1: armour is not siege", got)
	}
}
