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

	// Joining are units dispatched to the squad that have not reached it yet.
	//
	// They are deliberately NOT members. A unit walking from the base to a
	// squad already downrange is sixty cells behind by definition, and counting
	// it on dispatch guarantees the squad fails its own cohesion test: game 174
	// fielded twenty members with spread 60 and ZERO of them inside the clump
	// radius, which failed the gate, which cleared Attacking, which reset the
	// approach to step zero, which sent the squad back out to the detour
	// waypoint - a loop that swung it across 45% of the map diagonal.
	//
	// Three previous attempts at this all restricted WHO may join: freeze on
	// commitment, a fixed join radius, then a contact test. The error was never
	// who, it was WHEN they count. They count on arrival.
	Joining []int
}

// joinArrivedCells is how close a joiner must be to become a member. Twice the
// rally radius, so arriving does not immediately make the squad unclumped.
const joinArrivedCells = squadRallyRadius * 2

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

		// Joiners that reached the squad become members; the dead are dropped.
		if len(sq.Joining) > 0 {
			cx, cy, haveCentroid := squadCentroidOf(sq, env.State.Units)
			stillJoining := sq.Joining[:0]
			for _, id := range sq.Joining {
				if !aliveIDs[id] {
					continue
				}
				p, known := posNow[id]
				if haveCentroid && known {
					dx, dy := p[0]-cx, p[1]-cy
					if dx*dx+dy*dy <= joinArrivedCells*joinArrivedCells {
						sq.UnitIDs = append(sq.UnitIDs, id)
						continue
					}
				}
				if !haveCentroid {
					// Nothing to walk toward; treat them as arrived.
					sq.UnitIDs = append(sq.UnitIDs, id)
					continue
				}
				stillJoining = append(stillJoining, id)
			}
			sq.Joining = stillJoining
		}

		if len(sq.UnitIDs) == 0 && len(sq.Joining) == 0 {
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

// squadCentroidOf is the mean position of a squad's MEMBERS, ignoring joiners
// who by definition are not there yet.
func squadCentroidOf(sq *Squad, units []model.Unit) (int, int, bool) {
	ids := make(map[int]bool, len(sq.UnitIDs))
	for _, id := range sq.UnitIDs {
		ids[id] = true
	}
	sx, sy, n := 0, 0, 0
	for _, u := range units {
		if ids[u.ID] {
			sx += u.X
			sy += u.Y
			n++
		}
	}
	if n == 0 {
		return 0, 0, false
	}
	return sx / n, sy / n, true
}
