package rules

import "github.com/nstehr/vimy/vimy-core/model"

// Squad gives units persistent identity across ticks. Without squads, the AI
// would re-select units every tick and couldn't maintain coherent attack groups
// or reserve a defense force.
type Squad struct {
	Name    string // "attack-1", "defense", "scout"
	Domain  string // "ground", "air", "naval"
	UnitIDs []int  // persistent unit roster
	Role    string // "attack", "defend", "scout" — informational for LLM summary
}

func getSquads(memory map[string]any) map[string]*Squad {
	if v, ok := memory["squads"].(map[string]*Squad); ok {
		return v
	}
	return make(map[string]*Squad)
}

// GetSquads is the public accessor (used by strategist to summarize for LLM).
func GetSquads(memory map[string]any) map[string]*Squad {
	return getSquads(memory)
}

// updateSquads removes dead units each tick. Squads with no survivors
// are dissolved so formation rules can create fresh ones.
func updateSquads(env RuleEnv) {
	squads := getSquads(env.Memory)
	aliveIDs := makeUnitIDSet(env.State.Units)

	for name, sq := range squads {
		alive := sq.UnitIDs[:0]
		for _, id := range sq.UnitIDs {
			if aliveIDs[id] {
				alive = append(alive, id)
			}
		}
		sq.UnitIDs = alive

		if len(sq.UnitIDs) == 0 {
			delete(squads, name)
		}
	}
	env.Memory["squads"] = squads
}

func makeUnitIDSet(units []model.Unit) map[int]bool {
	s := make(map[int]bool, len(units))
	for _, u := range units {
		s[u.ID] = true
	}
	return s
}

// squadUnitIDSet is used by UnassignedIdle* to exclude squad members from the free pool.
func squadUnitIDSet(memory map[string]any) map[int]bool {
	squads := getSquads(memory)
	s := make(map[int]bool)
	for _, sq := range squads {
		for _, id := range sq.UnitIDs {
			s[id] = true
		}
	}
	return s
}
