package rules

import "github.com/nstehr/vimy/vimy-core/model"

// Squad gives units identity across ticks. Without it the AI re-selects from
// scratch each tick and can neither hold an attack group together nor keep a
// defense force in reserve.
type Squad struct {
	Name       string // "attack-1", "defense", "scout"
	Domain     string // "ground", "air", "naval"
	UnitIDs    []int  // persistent unit roster
	Role       string // "attack", "defend", "scout" — informational for LLM summary
	TargetSize int    // intended formation size; reinforcement tops up to this
}

func getSquads(memory map[string]any) map[string]*Squad {
	if v, ok := memory["squads"].(map[string]*Squad); ok {
		return v
	}
	return make(map[string]*Squad)
}

// GetSquads is the accessor for callers outside the package.
func GetSquads(memory map[string]any) map[string]*Squad {
	return getSquads(memory)
}

// updateSquads reaps dead units and dissolves emptied squads, so formation
// rules can build fresh ones.
func updateSquads(env RuleEnv) {
	squads := getSquads(env.Memory)
	aliveIDs := makeUnitIDSet(env.State.Units)

	// Where each member stood last tick. A unit that vanishes is already gone
	// from State.Units, so its position has to have been kept beforehand -
	// otherwise the only thing a death tells us is that it happened.
	posNow := make(map[int][2]int, len(env.State.Units))
	for _, u := range env.State.Units {
		posNow[u.ID] = [2]int{u.X, u.Y}
	}
	lastSeen, _ := env.Memory["squadLastSeen"].(map[int][2]int)
	if lastSeen == nil {
		lastSeen = map[int][2]int{}
	}

	for name, sq := range squads {
		alive := sq.UnitIDs[:0]
		for _, id := range sq.UnitIDs {
			if aliveIDs[id] {
				alive = append(alive, id)
				continue
			}
			// Lost. Remember where, so the threat field learns the spot even
			// though whatever killed it was never sighted.
			if p, ok := lastSeen[id]; ok {
				RecordDeathSite(env.Memory, p[0], p[1], env.State.Tick)
			}
		}
		sq.UnitIDs = alive

		if len(sq.UnitIDs) == 0 {
			delete(squads, name)
			delete(env.Memory, "huntBase:"+name)
		}
	}
	// Keep positions only for units still rostered, so the map tracks the
	// squads rather than growing for the whole game.
	kept := make(map[int][2]int)
	for _, sq := range squads {
		for _, id := range sq.UnitIDs {
			if p, ok := posNow[id]; ok {
				kept[id] = p
			} else if p, ok := lastSeen[id]; ok {
				kept[id] = p
			}
		}
	}
	env.Memory["squadLastSeen"] = kept
	env.Memory["squads"] = squads
}

func makeUnitIDSet(units []model.Unit) map[int]bool {
	s := make(map[int]bool, len(units))
	for _, u := range units {
		s[u.ID] = true
	}
	return s
}

// squadUnitIDSet is what keeps squad members out of the free pool.
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
