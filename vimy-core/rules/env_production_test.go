package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

// Game 192's exact shape: Soviet, both tanks buildable, cash enough for the
// 1150 heavy tank and not the 2000 mammoth.
//
// The heavy_tank role is the mammoth alone and medium_tank is [2tnk, 3tnk], so
// taking the first BUILDABLE type parked every order on a mammoth Vimy could
// afford in 15 of 699 sampled states. 169 tank orders, zero tanks built.
func TestAffordableTypePrefersWhatCanBePaidFor(t *testing.T) {
	soviet := func(cash int) RuleEnv {
		return RuleEnv{
			State: model.GameState{
				Player: model.Player{Cash: cash},
				ProductionQueues: []model.ProductionQueue{{
					Type:      QueueVehicle,
					Buildable: []string{"4tnk", "3tnk", "ftrk"},
					BuildableCosts: map[string]int{
						"4tnk": 2000, "3tnk": 1150, "ftrk": 600,
					},
				}},
			},
			Memory: map[string]any{},
		}
	}

	// Cash for the heavy tank but not the mammoth: heavy_tank yields nothing
	// affordable, so the caller falls through to medium_tank and gets the 3tnk.
	e := soviet(1200)
	if got := e.AffordableType("heavy_tank"); got != "" {
		t.Errorf("heavy_tank at 1200 cash = %q, want empty (a mammoth is 2000)", got)
	}
	if got := e.AffordableType("medium_tank"); got != "3tnk" {
		t.Errorf("medium_tank at 1200 cash = %q, want 3tnk", got)
	}

	// With the money, the mammoth is the right answer.
	if got := soviet(2500).AffordableType("heavy_tank"); got != "4tnk" {
		t.Errorf("heavy_tank at 2500 cash = %q, want 4tnk", got)
	}

	// Broke: nothing is affordable and the caller falls back to BuildableType,
	// which still names the heaviest so an order progresses as cash arrives.
	poor := soviet(100)
	if got := poor.AffordableType("heavy_tank"); got != "" {
		t.Errorf("heavy_tank at 100 cash = %q, want empty", got)
	}
	if got := poor.BuildableType("heavy_tank"); got != "4tnk" {
		t.Errorf("BuildableType must be unchanged: %q", got)
	}

	// An older mod build sends no costs at all; treat everything as affordable
	// rather than refusing to build.
	nocosts := soviet(100)
	nocosts.State.ProductionQueues[0].BuildableCosts = nil
	if got := nocosts.AffordableType("heavy_tank"); got != "4tnk" {
		t.Errorf("with no costs recorded = %q, want 4tnk", got)
	}
}

// A preference list is a WEIGHTED order: the head dominates, everything is built.
//
// Least-built cycling ignored position entirely -- the strategist asked for
// [artillery, medium_tank, light_tank] in 223 of 233 windows and game 219 built
// 20/8/24, the third choice most. Strict first-buildable order overcorrected:
// artillery and medium_tank both cost 850, so game 225 built 7/0/6 and the
// second entry became unreachable. This asserts the middle: repeated picks
// converge on the 3:2:1 the list position implies.
func TestPreferenceListConvergesOnItsPositionalWeights(t *testing.T) {
	counts := map[string]int{"arty": 0, "2tnk": 0, "1tnk": 0}
	role := map[string]string{"arty": "artillery", "2tnk": "medium_tank", "1tnk": "light_tank"}
	picks := map[string]int{}
	for i := 0; i < 60; i++ {
		units := []model.Unit{}
		id := 1
		for typ, n := range counts {
			for j := 0; j < n; j++ {
				units = append(units, model.Unit{ID: id, Type: typ})
				id++
			}
		}
		env := RuleEnv{
			Preferences: UnitPreferences{Vehicle: []string{"artillery", "medium_tank", "light_tank"}},
			State: model.GameState{
				Tick: 20000, Player: model.Player{Cash: 9000}, Units: units,
				ProductionQueues: []model.ProductionQueue{{
					Type: "Vehicle", Buildable: []string{"arty", "2tnk", "1tnk"},
					BuildableCosts: map[string]int{"arty": 850, "2tnk": 850, "1tnk": 700},
				}},
			},
		}
		got := env.BestBuildableVehicle()
		if got == "" {
			t.Fatalf("iteration %d picked nothing", i)
		}
		counts[got]++
		picks[role[got]]++
	}
	// 60 picks at 3:2:1 is 30/20/10. Allow slack for integer stepping.
	if picks["artillery"] < 25 || picks["artillery"] > 35 {
		t.Errorf("artillery picked %d of 60, want ~30 (weight 3 of 6): %v", picks["artillery"], picks)
	}
	if picks["medium_tank"] < 15 || picks["medium_tank"] > 25 {
		t.Errorf("medium_tank picked %d of 60, want ~20 (weight 2 of 6): %v", picks["medium_tank"], picks)
	}
	if picks["light_tank"] == 0 {
		t.Errorf("light_tank never picked; every entry in the list must be built: %v", picks)
	}
	if picks["artillery"] <= picks["medium_tank"] || picks["medium_tank"] <= picks["light_tank"] {
		t.Errorf("the order must be respected as a ranking: %v", picks)
	}
}

// A preference list is an ORDER, not a set to round-robin.
//
// The strategist set preferred_vehicle to [artillery, medium_tank, light_tank]
// in 223 of 233 doctrine windows across games 217-224 -- artillery first,
// unambiguously -- and least-built cycling built the THIRD choice as often as
// the first and the SECOND least of all. Game 219: 20 artillery, 8 medium, 24
// light. Game 220: 11, 3, 11. Position only broke ties, so the army came out as
// one or two of every vehicle type rather than a mass of the one asked for.
func TestPreferenceListIsHonouredAsAnOrder(t *testing.T) {
	// Everything buildable and affordable, and the head of the list is the one
	// we ALREADY have most of -- which is exactly when least-built cycling
	// would pick something else.
	env := RuleEnv{
		Preferences: UnitPreferences{Vehicle: []string{"artillery", "medium_tank", "light_tank"}},
		State: model.GameState{
			Tick: 20000, Player: model.Player{Cash: 5000},
			Units: []model.Unit{
				{ID: 1, Type: "arty"}, {ID: 2, Type: "arty"}, {ID: 3, Type: "arty"},
			},
			ProductionQueues: []model.ProductionQueue{{
				Type: "Vehicle", Buildable: []string{"arty", "2tnk", "1tnk"},
				BuildableCosts: map[string]int{"arty": 600, "2tnk": 800, "1tnk": 700},
			}},
		},
	}
	// Three artillery against weight 3 scores 1.0; medium and light at zero
	// score 0, so the under-built entries come first. That is the point of
	// weighting rather than strict order -- the head leads over time without
	// excluding the rest.
	if got := env.BestBuildableVehicle(); got == "arty" {
		t.Errorf("picked artillery with three already alive and none of the others; "+
			"weighting should top up the under-built entries, got %q", got)
	}
}

// An unaffordable head does not block a cheaper preference behind it: the
// affordable pass runs over the whole list before the buildable pass does.
// Without that, game 192's shape returns -- 169 tank orders, zero tanks built.
func TestUnaffordableHeadFallsThroughToTheNextPreference(t *testing.T) {
	env := RuleEnv{
		Preferences: UnitPreferences{Vehicle: []string{"medium_tank", "artillery"}},
		State: model.GameState{
			Tick: 20000, Player: model.Player{Cash: 650}, // enough for arty, not 2tnk
			ProductionQueues: []model.ProductionQueue{{
				Type: "Vehicle", Buildable: []string{"2tnk", "arty"},
				BuildableCosts: map[string]int{"2tnk": 800, "arty": 600},
			}},
		},
	}
	if got := env.BestBuildableVehicle(); got != "arty" {
		t.Errorf("picked %q; the head is unaffordable, so the next affordable "+
			"preference must win rather than stalling on it", got)
	}
}

// The FALLBACK enumeration keeps least-built cycling. combatVehicleRoles has no
// meaningful order, so when the doctrine has expressed no preference, spreading
// production is the right default and must not change.
func TestFallbackEnumerationStillCyclesLeastBuilt(t *testing.T) {
	env := RuleEnv{
		// No preferences at all, so BestBuildableVehicle falls through.
		State: model.GameState{
			Tick: 20000, Player: model.Player{Cash: 5000},
			Units: []model.Unit{{ID: 1, Type: "arty"}, {ID: 2, Type: "arty"}},
			ProductionQueues: []model.ProductionQueue{{
				Type: "Vehicle", Buildable: []string{"arty", "2tnk"},
				BuildableCosts: map[string]int{"arty": 600, "2tnk": 800},
			}},
		},
	}
	if got := env.BestBuildableVehicle(); got == "arty" {
		t.Error("with two artillery alive and no doctrine preference, the fallback " +
			"should cycle to the least-built role rather than repeat artillery")
	}
}
