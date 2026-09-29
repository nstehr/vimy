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
