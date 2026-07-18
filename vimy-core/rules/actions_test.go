package rules

import (
	"math"
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func TestIsCriticalRepairType(t *testing.T) {
	critical := []string{"fact", "weap", "tent", "barr", "proc", "powr", "apwr"}
	noncritical := []string{"dome", "atek", "stek", "afld", "hpad", "pbox", "sam"}
	for _, t1 := range critical {
		if !isCriticalRepairType(t1) {
			t.Errorf("expected %q to be critical", t1)
		}
	}
	for _, t1 := range noncritical {
		if isCriticalRepairType(t1) {
			t.Errorf("expected %q to be non-critical", t1)
		}
	}
}

func TestDefenseHint_NoBuildings(t *testing.T) {
	env := RuleEnv{
		State:  model.GameState{},
		Memory: make(map[string]any),
	}
	x, y := defenseHint(env)
	if x != 0 || y != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", x, y)
	}
}

func TestDefenseHint_SingleBuilding(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100},
			},
		},
		Memory: make(map[string]any),
	}
	// With a single building, radius clamps to 3, and candidates are placed
	// around the perimeter. Result must be near the building.
	for range 20 {
		x, y := defenseHint(env)
		dx := math.Abs(float64(x - 100))
		dy := math.Abs(float64(y - 100))
		if dx > 10 || dy > 10 {
			t.Errorf("hint (%d,%d) too far from single building at (100,100)", x, y)
		}
	}
}

func TestDefenseHint_ThreatBias(t *testing.T) {
	// Enemy base is to the east (X=500). Defenses should bias eastward.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100},
				{ID: 2, Type: "powr", X: 80, Y: 120},
				{ID: 3, Type: "proc", X: 120, Y: 80},
			},
		},
		Memory: make(map[string]any),
	}
	// Seed enemy base intel.
	env.Memory["enemyBases"] = map[string]EnemyBaseIntel{
		"Enemy": {Owner: "Enemy", X: 500, Y: 100, Tick: 1, FromBuildings: true},
	}

	eastCount, westCount := 0, 0
	cx := 100 // approximate centroid
	for range 200 {
		x, _ := defenseHint(env)
		if x > cx {
			eastCount++
		} else {
			westCount++
		}
	}
	// With threat to the east, we expect a strong eastward bias.
	if eastCount <= westCount {
		t.Errorf("expected eastward bias: east=%d west=%d", eastCount, westCount)
	}
}

func TestDefenseHint_SpreadFromExisting(t *testing.T) {
	// Place an existing defense at the north of the base. New defenses
	// should generally avoid clustering there.
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100},
				{ID: 2, Type: "powr", X: 80, Y: 120},
				{ID: 3, Type: "powr", X: 120, Y: 120},
				{ID: 4, Type: "pbox", X: 100, Y: 80}, // existing defense, north
			},
		},
		Memory: make(map[string]any),
	}

	northCount, otherCount := 0, 0
	cy := 105 // approximate centroid
	for range 200 {
		_, y := defenseHint(env)
		if y < cy-10 {
			northCount++
		} else {
			otherCount++
		}
	}
	// Spread factor should discourage clustering near the existing defense.
	// Other directions should dominate.
	if northCount > otherCount {
		t.Errorf("expected spread away from existing defense: north=%d other=%d", northCount, otherCount)
	}
}

func TestDefenseHint_HarvesterEmergencyBiasesToRefinery(t *testing.T) {
	// Main base at (100,100). Refinery far out at (400,400) — periphery.
	// Without harvester emergency, threat-toward-enemy dominates and defenses
	// land near the main cluster. With 2+ harvesters fleeing, defenses should
	// shift toward the refinery (large positive X AND Y).
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 100, Y: 100},
				{ID: 2, Type: "powr", X: 80, Y: 120},
				{ID: 3, Type: "proc", X: 400, Y: 400}, // remote refinery
			},
		},
		Memory: make(map[string]any),
	}
	env.Memory["enemyBases"] = map[string]EnemyBaseIntel{
		"Enemy": {Owner: "Enemy", X: 500, Y: 100, Tick: 1, FromBuildings: true},
	}
	// Simulate 2 fleeing harvesters via the existing flee-state map.
	env.Memory["harvesterFleeing"] = map[int]struct {
		Tick, X, Y int
	}{
		1: {Tick: 1, X: 380, Y: 380},
		2: {Tick: 1, X: 420, Y: 420},
	}

	nearRefinery, elsewhere := 0, 0
	for range 200 {
		x, y := defenseHint(env)
		dx, dy := x-400, y-400
		if dx*dx+dy*dy < 200*200 { // within 200 of the refinery
			nearRefinery++
		} else {
			elsewhere++
		}
	}
	if nearRefinery <= elsewhere {
		t.Errorf("expected defense placement bias to refinery during harvester emergency: nearRefinery=%d elsewhere=%d", nearRefinery, elsewhere)
	}
}

func TestDefenseHint_AllWaterFallback(t *testing.T) {
	// Terrain grid that is entirely water — should fall back to centroid.
	grid := &model.TerrainGrid{
		Cols:  4,
		Rows:  4,
		CellW: 100,
		CellH: 100,
		Grid:  make([]model.TerrainType, 16),
	}
	for i := range grid.Grid {
		grid.Grid[i] = model.Water
	}

	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 200, Y: 200},
				{ID: 2, Type: "powr", X: 180, Y: 220},
			},
		},
		Memory:  make(map[string]any),
		Terrain: grid,
	}

	x, y := defenseHint(env)
	// Should fall back to centroid (190, 210).
	cx, cy := 190, 210
	dx := math.Abs(float64(x - cx))
	dy := math.Abs(float64(y - cy))
	if dx > 1 || dy > 1 {
		t.Errorf("expected centroid fallback (~%d,~%d), got (%d,%d)", cx, cy, x, y)
	}
}

func TestDefenseHint_NoEnemyIntel(t *testing.T) {
	// Without enemy intel, threat is neutral. Results should still be
	// distributed around the perimeter (no crash, reasonable positions).
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 200, Y: 200},
				{ID: 2, Type: "proc", X: 160, Y: 240},
				{ID: 3, Type: "weap", X: 240, Y: 160},
			},
		},
		Memory: make(map[string]any),
	}

	for range 50 {
		x, y := defenseHint(env)
		// Should be in the general vicinity of the base, not wildly off.
		dx := math.Abs(float64(x - 200))
		dy := math.Abs(float64(y - 200))
		if dx > 200 || dy > 200 {
			t.Errorf("hint (%d,%d) unreasonably far from base centroid", x, y)
		}
	}
}

func TestDefenseHint_PullsTowardChokepoint(t *testing.T) {
	// Base sits in a pocket of land. A bridge at zone (3,2) — map ~(350,250)
	// — is the only crossing to the enemy side. Defenses should cluster near
	// that bridge far more often than on the opposite side of the base.
	grid := &model.TerrainGrid{
		Cols: 7, Rows: 5, CellW: 100, CellH: 100,
		Grid: []model.TerrainType{
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Water, model.Water, model.Water, model.Bridge, model.Water, model.Water, model.Water,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
			model.Land, model.Land, model.Land, model.Land, model.Land, model.Land, model.Land,
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{ID: 1, Type: "fact", X: 300, Y: 100},
				{ID: 2, Type: "powr", X: 280, Y: 120},
				{ID: 3, Type: "proc", X: 320, Y: 80},
			},
		},
		Memory:  make(map[string]any),
		Terrain: grid,
	}
	env.Memory["enemyBases"] = map[string]EnemyBaseIntel{
		"Enemy": {Owner: "Enemy", X: 300, Y: 400, Tick: 1, FromBuildings: true},
	}

	// Bridge zone center: (350, 250). "Near" = within one cell of it.
	nearBridge, awayFromBridge := 0, 0
	for range 300 {
		x, y := defenseHint(env)
		dxB := math.Abs(float64(x - 350))
		dyB := math.Abs(float64(y - 250))
		if dxB <= 120 && dyB <= 120 {
			nearBridge++
		} else if y < 100 {
			// Directly opposite the bridge, north of the base.
			awayFromBridge++
		}
	}
	if nearBridge <= awayFromBridge {
		t.Errorf("expected placements to favor bridge: near=%d away=%d", nearBridge, awayFromBridge)
	}

	// Critical: defenses must never land on the bridge zone itself — a
	// pillbox on the only crossing would strand our own units.
	onBridge := 0
	for range 300 {
		x, y := defenseHint(env)
		if grid.AtMapPos(x, y) == model.Bridge {
			onBridge++
		}
	}
	if onBridge > 0 {
		t.Errorf("defense placed on bridge tile %d/300 times — would block our own movement", onBridge)
	}
}

// --- ActionUnloadAPCNearTarget ---

// Verify unload threshold is permissive enough to survive APC pathing stalls.
// The previous 5-cell threshold was too tight and the engineer never disembarked.
func TestActionUnloadAPCNearTarget_UnloadsAtNewThreshold(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Tick: 100,
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 108, Y: 100},
			},
			Capturables: []model.Enemy{
				{ID: 200, Type: "oilb", Owner: "Neutral", X: 100, Y: 100},
			},
		},
		Memory: map[string]any{
			"apcCargoIntent": map[int]string{10: apcIntentEngineer},
		},
	}

	// Distance ≈ 8, within the new 10-cell threshold. Should send an Unload.
	if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tracking map should be cleared after unload.
	progress := getAPCProgress(env.Memory)
	if _, ok := progress[10]; ok {
		t.Error("expected apcDeliveryProgress cleared after unload")
	}
}

// APC stuck at the same tile beyond apcStallTicks must unload even if still
// farther than the normal unload threshold — stall is the fallback path.
func TestActionUnloadAPCNearTarget_UnloadsOnStall(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Tick: 1000,
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
			Capturables: []model.Enemy{
				{ID: 200, Type: "oilb", Owner: "Neutral", X: 100, Y: 100}, // 70+ cells away
			},
		},
		Memory: map[string]any{
			"apcDeliveryProgress": map[int]apcProgressEntry{
				// Hasn't moved for >apcStallTicks. Use a generous gap so this
				// test stays valid if the threshold gets bumped further.
				10: {LastTick: 500, LastX: 50, LastY: 50},
			},
			"apcCargoIntent": map[int]string{10: apcIntentEngineer},
		},
	}

	if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tracking map should be cleared after unload.
	progress := getAPCProgress(env.Memory)
	if _, ok := progress[10]; ok {
		t.Error("expected apcDeliveryProgress cleared after stall-triggered unload")
	}
}

// A brief non-movement window (e.g. server hasn't started pathing yet) must
// NOT trigger a premature stall-unload. This is the original bug: 50 ticks
// was too short, so APCs unloaded at base before ever reaching the target.
func TestActionUnloadAPCNearTarget_DoesNotStallPrematurely(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Tick: 200,
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
			Capturables: []model.Enemy{
				{ID: 200, Type: "oilb", Owner: "Neutral", X: 500, Y: 500}, // far
			},
		},
		Memory: map[string]any{
			"apcDeliveryProgress": map[int]apcProgressEntry{
				// Only ~100 ticks without movement — normal startup/pathing window.
				10: {LastTick: 100, LastX: 50, LastY: 50},
			},
			"apcCargoIntent": map[int]string{10: apcIntentEngineer},
		},
	}

	if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Tag must survive — we sent a move, not an unload.
	if GetAPCCargoIntent(env.Memory)[10] != apcIntentEngineer {
		t.Error("premature stall: APC was unloaded within the pathing-startup window")
	}
}

// Moving APC — tracking should update so stall detection stays accurate.
func TestActionUnloadAPCNearTarget_TracksProgressWhileMoving(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Tick: 500,
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
			Capturables: []model.Enemy{
				{ID: 200, Type: "oilb", Owner: "Neutral", X: 100, Y: 100},
			},
		},
		Memory: map[string]any{
			"apcDeliveryProgress": map[int]apcProgressEntry{
				10: {LastTick: 490, LastX: 40, LastY: 40}, // was elsewhere last tick
			},
			"apcCargoIntent": map[int]string{10: apcIntentEngineer},
		},
	}

	if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	progress := getAPCProgress(env.Memory)
	entry, ok := progress[10]
	if !ok {
		t.Fatal("expected tracking entry for APC 10")
	}
	if entry.LastX != 50 || entry.LastY != 50 || entry.LastTick != 500 {
		t.Errorf("expected updated tracking to (50,50,500), got (%d,%d,%d)", entry.LastX, entry.LastY, entry.LastTick)
	}
}

// Move commands must be throttled — re-issuing the same destination each tick
// cancels the in-flight path in OpenRA and pins the APC in place. Observed live:
// 473 Move(123,123) sent over a 13-minute game with actor moving only 14 cells.
func TestActionUnloadAPCNearTarget_ThrottlesMoveSpam(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	mkEnv := func(tick int, memory map[string]any) RuleEnv {
		return RuleEnv{
			State: model.GameState{
				Tick: tick,
				Units: []model.Unit{
					{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
				},
				Capturables: []model.Enemy{
					{ID: 200, Type: "oilb", Owner: "Neutral", X: 100, Y: 100},
				},
			},
			Memory: memory,
		}
	}

	// 1. First call seeds the throttle state with the current tick.
	mem := map[string]any{"apcCargoIntent": map[int]string{10: apcIntentEngineer}}
	env := mkEnv(500, mem)
	if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
		t.Fatalf("first call: %v", err)
	}
	state := getAPCMoveState(mem)
	entry, ok := state[10]
	if !ok || entry.Tick != 500 || entry.X != 100 || entry.Y != 100 {
		t.Fatalf("expected throttle entry (tick=500, 100,100), got %+v ok=%v", entry, ok)
	}

	// 2. Same destination, within apcMoveResend window → must NOT resend (tick frozen).
	env2 := mkEnv(500+apcMoveResend-1, mem)
	if err := ActionUnloadAPCNearTarget(env2, conn); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if state[10].Tick != 500 {
		t.Errorf("expected throttle entry tick to stay at 500 (suppressed resend), got %d", state[10].Tick)
	}

	// 3. Same destination but past the resend window → must resend (tick advances).
	env3 := mkEnv(500+apcMoveResend+1, mem)
	if err := ActionUnloadAPCNearTarget(env3, conn); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if state[10].Tick != 500+apcMoveResend+1 {
		t.Errorf("expected throttle entry tick to advance past window, got %d", state[10].Tick)
	}
}

// With no visible capturable, the loaded APC should go explore, not no-op.
// This is the scout-with-loaded-APC behaviour aggressive-capture doctrines rely on.
func TestActionUnloadAPCNearTarget_ExploresWhenNoCapturable(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Tick:      100,
			MapWidth:  1000,
			MapHeight: 1000,
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
			// no Capturables
		},
		Memory: map[string]any{
			"apcCargoIntent": map[int]string{10: apcIntentEngineer},
		},
	}

	if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	targets := getAPCExploreTargets(env.Memory)
	entry, ok := targets[10]
	if !ok {
		t.Fatal("expected exploration target for APC 10")
	}
	// Farthest-waypoint heuristic should pick a point across the map from (50,50).
	if entry.X < 500 && entry.Y < 500 {
		t.Errorf("expected exploration target across the map, got (%d,%d)", entry.X, entry.Y)
	}
}

// Re-invoking with the same APC far from its current target must keep the
// waypoint index — otherwise the APC would pick a new destination every tick
// and never reach any of them. With round-robin patrol the waypoint index is
// authoritative; X,Y are derived from it each call.
func TestActionUnloadAPCNearTarget_ExploreTargetSticky(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// MapWidth 1000 → generateWaypoints yields a 9-point grid with margins at
	// 40/960 and midpoints at 500. waypoints[2] = (maxX, minY) = (960, 40).
	env := RuleEnv{
		State: model.GameState{
			Tick:      200,
			MapWidth:  1000,
			MapHeight: 1000,
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
		},
		Memory: map[string]any{
			"apcExploreTarget": map[int]apcExploreEntry{
				10: {X: 960, Y: 40, AssignedAt: 100, Idx: 2},
			},
			"apcCargoIntent": map[int]string{10: apcIntentEngineer},
		},
	}

	if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entry := getAPCExploreTargets(env.Memory)[10]
	if entry.Idx != 2 {
		t.Errorf("expected idx preserved at 2 (APC still en route), got %d", entry.Idx)
	}
	if entry.X != 960 || entry.Y != 40 {
		t.Errorf("expected waypoint (960,40) preserved, got (%d,%d)", entry.X, entry.Y)
	}
	if entry.AssignedAt != 100 {
		t.Errorf("expected AssignedAt preserved at 100 for TTL accounting, got %d", entry.AssignedAt)
	}
}

// --- APC cargo disambiguation ---

// An engineer-tagged APC must not be picked up by the combat-loaded list, and
// vice versa. This is the core invariant for split-intent delivery rules.
func TestAPCCargoIntent_FiltersByTag(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1},
				{ID: 11, Type: "apc", Idle: true, CargoCount: 1},
				{ID: 12, Type: "apc", Idle: true, CargoCount: 1}, // untagged
			},
		},
		Memory: map[string]any{
			"apcCargoIntent": map[int]string{
				10: apcIntentEngineer,
				11: apcIntentCombat,
			},
		},
	}

	eng := env.IdleEngineerLoadedAPCs()
	if len(eng) != 1 || eng[0].ID != 10 {
		t.Errorf("IdleEngineerLoadedAPCs: expected only APC 10, got %+v", eng)
	}

	combat := env.IdleCombatLoadedAPCs()
	if len(combat) != 1 || combat[0].ID != 11 {
		t.Errorf("IdleCombatLoadedAPCs: expected only APC 11, got %+v", combat)
	}
}

// Stale tags for dead APCs must be cleaned up. Empty (but live) APCs keep
// their tag — an APC that just had a load command sent shows CargoCount=0
// for a tick or two before the server processes the Enter, and pruning
// during that window strands the APC forever (no deliver rule will pick
// it up once CargoCount finally increments because the tag is gone).
func TestAPCCargoIntent_PrunesStaleTags(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1}, // live + loaded
				{ID: 11, Type: "apc", Idle: true, CargoCount: 0}, // load in flight (or just unloaded)
				// APC 12 is gone from the map entirely
			},
		},
		Memory: map[string]any{
			"apcCargoIntent": map[int]string{
				10: apcIntentEngineer,
				11: apcIntentEngineer,
				12: apcIntentCombat,
			},
		},
	}

	_ = env.IdleEngineerLoadedAPCs() // triggers prune
	intent := GetAPCCargoIntent(env.Memory)
	if _, ok := intent[11]; !ok {
		t.Error("APC 11 tag must survive CargoCount=0 transient (load in flight)")
	}
	if _, ok := intent[12]; ok {
		t.Error("expected dead APC 12 tag to be pruned")
	}
	if intent[10] != apcIntentEngineer {
		t.Errorf("expected APC 10 tag preserved, got %q", intent[10])
	}
}

// After unload, the tag must be explicitly cleared so the APC can pick up a
// new mission (e.g. get re-tagged combat by a later load-assault-infantry).
func TestAPCCargoIntent_ClearedOnUnload(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Tick: 100,
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 108, Y: 100},
			},
			Capturables: []model.Enemy{
				{ID: 200, Type: "oilb", Owner: "Neutral", X: 100, Y: 100},
			},
		},
		Memory: map[string]any{
			"apcCargoIntent": map[int]string{10: apcIntentEngineer},
		},
	}

	if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := GetAPCCargoIntent(env.Memory)[10]; ok {
		t.Error("expected cargo intent cleared after engineer-path unload")
	}
}

// Assault unload must also clear the cargo intent tag.
func TestAPCCargoIntent_ClearedOnAssaultUnload(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{{ID: 100, Type: "fact", X: 50, Y: 50}},
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 500, Y: 503},
			},
		},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy": {Owner: "Enemy", X: 500, Y: 500, Tick: 1, FromBuildings: true},
			},
			"apcCargoIntent": map[int]string{10: apcIntentCombat},
		},
	}

	if err := ActionDeliverAssaultAPC(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := GetAPCCargoIntent(env.Memory)[10]; ok {
		t.Error("expected cargo intent cleared after assault unload")
	}
}

// Engineer tag is sticky within a tick: both load rules target the same APC
// but load-engineer fires first (priority 845 > 838). Once an engineer claims
// the APC, load-assault-infantry must NOT overwrite the tag, or the capture
// deliverer will lose the APC to the assault deliverer (which then sits idle
// until HasEnemyIntel()).
func TestLoadCombatInfantry_DoesNotOverwriteEngineerTag(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Simulate the order the engine would run: engineer load already tagged
	// APC 10 this tick; combat load now runs and must leave the tag alone.
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 2, Type: "e1", Idle: true},                  // rifle
				{ID: 10, Type: "apc", Idle: true, CargoCount: 0}, // same APC
			},
		},
		Memory: map[string]any{
			"apcCargoIntent": map[int]string{10: apcIntentEngineer},
		},
	}
	if err := ActionLoadCombatInfantry(env, conn); err != nil {
		t.Fatalf("ActionLoadCombatInfantry: %v", err)
	}
	if GetAPCCargoIntent(env.Memory)[10] != apcIntentEngineer {
		t.Errorf("engineer tag must be sticky; got %q", GetAPCCargoIntent(env.Memory)[10])
	}
}

// The load actions must write the intent tag so downstream deliver rules
// can disambiguate. This test drives both actions and checks the tag.
func TestLoadActions_TagAPCCargoIntent(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Engineer load tags apcIntentEngineer.
	engEnv := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "e6", Idle: true},                  // engineer
				{ID: 10, Type: "apc", Idle: true, CargoCount: 0}, // empty APC
			},
		},
		Memory: make(map[string]any),
	}
	if err := ActionLoadEngineerIntoAPC(engEnv, conn); err != nil {
		t.Fatalf("ActionLoadEngineerIntoAPC: %v", err)
	}
	if GetAPCCargoIntent(engEnv.Memory)[10] != apcIntentEngineer {
		t.Errorf("expected APC 10 tagged %q, got %q", apcIntentEngineer, GetAPCCargoIntent(engEnv.Memory)[10])
	}

	// Combat load tags apcIntentCombat.
	combatEnv := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 2, Type: "e1", Idle: true},                  // rifle infantry
				{ID: 20, Type: "apc", Idle: true, CargoCount: 0}, // empty APC
			},
		},
		Memory: make(map[string]any),
	}
	if err := ActionLoadCombatInfantry(combatEnv, conn); err != nil {
		t.Fatalf("ActionLoadCombatInfantry: %v", err)
	}
	if GetAPCCargoIntent(combatEnv.Memory)[20] != apcIntentCombat {
		t.Errorf("expected APC 20 tagged %q, got %q", apcIntentCombat, GetAPCCargoIntent(combatEnv.Memory)[20])
	}
}

// Soviets don't have Rangers; their scout fallback is a designated attack dog
// (first dog out of the kennel, human pattern). Without this, engineer-rush
// Soviet doctrines had zero non-APC scouting because IdleScouts() was always
// empty.
func TestDesignateScout_FallsBackToAttackDog(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				// No ranger, no light tank. Two dogs: scout should take the first.
				{ID: 11, Type: "dog", Idle: true, X: 5, Y: 5},
				{ID: 12, Type: "dog", Idle: true, X: 6, Y: 5},
			},
		},
		Memory: make(map[string]any),
	}
	designateScout(env)
	if getScoutID(env.Memory) != 11 {
		t.Errorf("expected first attack dog (id=11) designated as scout, got %d", getScoutID(env.Memory))
	}
	// IdleScouts should surface the designated dog.
	scouts := env.IdleScouts()
	if len(scouts) != 1 || scouts[0].ID != 11 {
		t.Errorf("expected IdleScouts to return [{11}], got %+v", scouts)
	}
}

func TestDesignateScout_PrefersLightTankOverDog(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 20, Type: "dog", Idle: true},
				{ID: 21, Type: "1tnk", Idle: true},
			},
		},
		Memory: make(map[string]any),
	}
	designateScout(env)
	if getScoutID(env.Memory) != 21 {
		t.Errorf("expected light tank (id=21) designated as scout, got %d", getScoutID(env.Memory))
	}
}

// Squad Attack orders must be throttled. Without this, SquadFocusFire
// re-issued identical Attack(actor→target) every tick and each new order
// cancelled the in-flight attack activity, pinning units mid-map. Observed:
// 52 identical (actor, target) pairs across 7 squad members in a single game.
func TestSendAttack_ThrottlesIdenticalOrders(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	mem := map[string]any{}
	env := func(tick int) RuleEnv {
		return RuleEnv{State: model.GameState{Tick: tick}, Memory: mem}
	}

	if err := sendAttack(env(100), conn, 10, 500); err != nil {
		t.Fatalf("first call: %v", err)
	}
	state := memoryMap[int, attackOrderEntry](mem, "attackOrderSent")
	if state[10].Tick != 100 {
		t.Fatalf("expected seed tick 100, got %+v", state[10])
	}

	// Same target inside window → suppressed.
	if err := sendAttack(env(100+attackOrderResend-1), conn, 10, 500); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if state[10].Tick != 100 {
		t.Errorf("expected tick frozen inside throttle window, got %d", state[10].Tick)
	}

	// Different target → immediate resend.
	if err := sendAttack(env(100+attackOrderResend-1), conn, 10, 501); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if state[10].TargetID != 501 {
		t.Errorf("expected target change to bypass throttle, got %+v", state[10])
	}
}

// sendAttackMove must suppress batch AttackMove resends for actors whose
// throttle windows haven't expired, while still letting new actors in the
// batch through. This prevents squad AttackMove from cancelling mid-flight
// pathing every tick.
func TestSendAttackMove_ThrottlesIdenticalAndAllowsNew(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	mem := map[string]any{}
	env := func(tick int) RuleEnv {
		return RuleEnv{State: model.GameState{Tick: tick}, Memory: mem}
	}

	// Seed batch order for actors [10, 11] at (50, 50).
	if err := sendAttackMove(env(1000), conn, []uint32{10, 11}, 50, 50); err != nil {
		t.Fatalf("first call: %v", err)
	}
	state := memoryMap[int, attackMoveEntry](mem, "attackMoveSent")
	if state[10].Tick != 1000 || state[11].Tick != 1000 {
		t.Fatalf("expected both actors seeded at tick 1000, got %+v / %+v", state[10], state[11])
	}

	// Same dest within window: no new send, both ticks frozen.
	if err := sendAttackMove(env(1000+attackMoveResend-1), conn, []uint32{10, 11}, 50, 50); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if state[10].Tick != 1000 || state[11].Tick != 1000 {
		t.Errorf("expected all ticks frozen inside window, got %+v / %+v", state[10], state[11])
	}

	// Actor 12 added to batch with same dest — fresh send for actor 12 only.
	if err := sendAttackMove(env(1000+attackMoveResend-1), conn, []uint32{10, 11, 12}, 50, 50); err != nil {
		t.Fatalf("mixed call: %v", err)
	}
	if _, ok := state[12]; !ok {
		t.Errorf("expected actor 12 throttle state to be recorded")
	}
	if state[10].Tick != 1000 {
		t.Errorf("actor 10 should remain suppressed, got %+v", state[10])
	}
}

func TestAttackMoveHasFreshTarget(t *testing.T) {
	mem := map[string]any{
		"attackMoveSent": map[int]attackMoveEntry{
			10: {Tick: 100, X: 50, Y: 50},
		},
	}
	env := RuleEnv{State: model.GameState{Tick: 120}, Memory: mem}

	// Actor 10 inside cooldown, same dest → no fresh target.
	if attackMoveHasFreshTarget(env, []uint32{10}, 50, 50) {
		t.Error("expected no fresh target when actor 10 is throttled")
	}
	// Different destination → fresh.
	if !attackMoveHasFreshTarget(env, []uint32{10}, 60, 60) {
		t.Error("expected fresh target when destination differs")
	}
	// New actor 20 not yet tracked → fresh.
	if !attackMoveHasFreshTarget(env, []uint32{20}, 50, 50) {
		t.Error("expected fresh target for untracked actor")
	}
}

// Minelayers that successfully placed their minefield must have their
// assignment cleared so ActionLayMines can re-task them to the next
// chokepoint. Without this, a minelayer stayed flagged `assigned` forever
// after mining and sat parked on its own minefield for the rest of the game.
func TestUpdateMinelayers_ClearsOnMissionComplete(t *testing.T) {
	// Minelayer 100 is assigned, alive, idle, and sitting at its target.
	// Expect: both maps cleared so IdleMinelayers returns it next tick.
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 100, Type: "mnly", Idle: true, X: 50, Y: 50},
			},
		},
		Memory: map[string]any{
			"minelayerAssigned": map[int]bool{100: true},
			"minelayerTargets": map[int]minelayerTarget{
				100: {X: 50, Y: 50, AssignedAt: 1000},
			},
		},
	}
	updateMinelayers(env)
	if env.Memory["minelayerAssigned"].(map[int]bool)[100] {
		t.Error("expected assigned=false after mission complete")
	}
	if _, ok := env.Memory["minelayerTargets"].(map[int]minelayerTarget)[100]; ok {
		t.Error("expected minelayerTargets cleared after mission complete")
	}
}

func TestUpdateMinelayers_KeepsAssignmentWhileEnRoute(t *testing.T) {
	// Minelayer 200 is assigned, alive, idle, but NOT at target yet (en
	// route, briefly idle between activities). Must keep assignment.
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 200, Type: "mnly", Idle: true, X: 20, Y: 20},
			},
		},
		Memory: map[string]any{
			"minelayerAssigned": map[int]bool{200: true},
			"minelayerTargets": map[int]minelayerTarget{
				200: {X: 80, Y: 80, AssignedAt: 1000},
			},
		},
	}
	updateMinelayers(env)
	if !env.Memory["minelayerAssigned"].(map[int]bool)[200] {
		t.Error("expected assignment preserved while minelayer still en route to target")
	}
}

// Idle-unit scouting must not spam AttackMove. Live game: 772 attack_move
// commands cycling units through corners every tick, cancelling each
// in-flight path. After the fix: identical destination within scoutMoveResend
// is suppressed; stall triggers round-robin advance.
func TestActionScoutWithIdleUnits_ThrottlesAndAdvancesOnStall(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	mkEnv := func(tick int, mem map[string]any) RuleEnv {
		return RuleEnv{
			State: model.GameState{
				Tick:      tick,
				MapWidth:  128,
				MapHeight: 128,
				Units: []model.Unit{
					// Position far from every waypoint so arrive-advance doesn't
					// fire between test steps. Nearest waypoint (5,5) is >35
					// cells away — well outside scoutArriveRadius.
					{ID: 50, Type: "3tnk", Idle: true, X: 30, Y: 30},
				},
			},
			Memory: mem,
		}
	}

	// 1. First call seeds a patrol assignment + throttle entry.
	mem := map[string]any{}
	if err := ActionScoutWithIdleUnits(mkEnv(1000, mem), conn); err != nil {
		t.Fatalf("first call: %v", err)
	}
	state := memoryMap[int, scoutMoveEntry](mem, "idleScoutMoveSent")
	first := state[50]
	if first.Tick != 1000 {
		t.Fatalf("expected seed tick=1000, got %+v", first)
	}

	// 2. Same tick-window, same position → suppressed (tick frozen).
	if err := ActionScoutWithIdleUnits(mkEnv(1000+scoutMoveResend-1, mem), conn); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if state[50].Tick != 1000 {
		t.Errorf("expected tick frozen at 1000 inside throttle window, got %d", state[50].Tick)
	}

	// 3. Simulate stall: unit still at (30,30), and stall tracking marks it
	// as having been there since tick 1000. Expect idx to advance. (Progress
	// tracking uses the unit's *current* position — overwrite to match the
	// test's fixed unit pos so actorStalledAt sees a genuine stall.)
	prevIdx := state[50].Idx
	progress := memoryMap[int, apcProgressEntry](mem, "idleScoutProgress")
	progress[50] = apcProgressEntry{LastTick: 1000, LastX: 30, LastY: 30}
	if err := ActionScoutWithIdleUnits(mkEnv(1000+scoutStallTicks+5, mem), conn); err != nil {
		t.Fatalf("stall call: %v", err)
	}
	if state[50].Idx == prevIdx {
		t.Errorf("expected idx to advance on stall, still at %d", prevIdx)
	}
}

// A scout parked on an unreachable waypoint (chokepoint / terrain lock) must
// advance to the next waypoint after scoutStallTicks, not retry the same
// destination forever. Live scenario: dog received 93 Move(5,5) commands from
// (64,64) across an entire game without moving a cell.
func TestActionScoutPatrol_StallAdvancesToNextWaypoint(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Scout 30 assigned to idx 1 → (5,5). Has been sitting at (64,64) since
	// tick 50; current tick 300 → 250 ticks stalled (> scoutStallTicks=200).
	// Expect advance to idx 2.
	mem := map[string]any{
		"scoutUnitID": 30,
		"scoutMoveSent": map[int]scoutMoveEntry{
			30: {Tick: 60, X: 5, Y: 5, Idx: 1},
		},
		"scoutProgress": map[int]apcProgressEntry{
			30: {LastTick: 50, LastX: 64, LastY: 64},
		},
	}
	env := RuleEnv{
		State: model.GameState{
			Tick:      300,
			MapWidth:  128,
			MapHeight: 128,
			Units: []model.Unit{
				{ID: 30, Type: "dog", Idle: true, X: 64, Y: 64},
			},
		},
		Memory: mem,
	}

	if err := ActionScoutPatrol(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := mem["scoutMoveSent"].(map[int]scoutMoveEntry)[30]
	if got.Idx != 2 {
		t.Errorf("expected idx to advance from 1 to 2 on stall, got %d", got.Idx)
	}
	if got.X == 5 && got.Y == 5 {
		t.Errorf("expected destination to change away from unreachable (5,5), still there")
	}
}

// ActionScoutPatrol must throttle Move re-sends. Without this, each tick
// issued a fresh Move per scout, cancelling the in-flight path every tick.
func TestActionScoutPatrol_ThrottlesMoveSpam(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	mkEnv := func(tick int, mem map[string]any) RuleEnv {
		return RuleEnv{
			State: model.GameState{
				Tick:      tick,
				MapWidth:  128,
				MapHeight: 128,
				Units: []model.Unit{
					{ID: 30, Type: "dog", Idle: true, X: 10, Y: 10},
				},
			},
			Memory: mem,
		}
	}
	mem := map[string]any{"scoutUnitID": 30}

	if err := ActionScoutPatrol(mkEnv(100, mem), conn); err != nil {
		t.Fatalf("first call: %v", err)
	}
	state := getScoutMoveState(mem)
	first := state[30]
	if first.Tick != 100 {
		t.Fatalf("expected seed tick=100, got %+v", first)
	}

	// Same destination within window → suppressed (waypoint index shouldn't
	// advance and tick stays put).
	if err := ActionScoutPatrol(mkEnv(100+scoutMoveResend-1, mem), conn); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if state[30].Tick != 100 {
		t.Errorf("expected tick to remain 100 inside window, got %d", state[30].Tick)
	}

	// Past the window → next waypoint gets sent.
	if err := ActionScoutPatrol(mkEnv(100+scoutMoveResend+1, mem), conn); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if state[30].Tick == 100 {
		t.Errorf("expected tick to advance past window, still 100")
	}
}

// A scouting APC stuck on an unreachable waypoint (e.g. corner blocked by a
// chokepoint) must re-pick a different waypoint. Observed live: 140 Move
// commands to (123,123), APC crawled 3 cells in 7 minutes because the
// deterministic farthest-first pick kept returning the same unreachable
// corner every TTL window.
func TestActionUnloadAPCNearTarget_StalledScoutRepicksDifferentWaypoint(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Map big enough to yield multiple candidate waypoints. APC at (27,38)
	// with a prior stale target assigned to the far corner — same position
	// observed in the live game.
	prevEntry := apcExploreEntry{X: 123, Y: 123, AssignedAt: 0}

	env := RuleEnv{
		State: model.GameState{
			Tick:      300, // > apcScoutStallTicks after the progress entry below
			MapWidth:  128,
			MapHeight: 128,
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 27, Y: 38},
			},
			// no Capturables — force scout branch
		},
		Memory: map[string]any{
			"apcCargoIntent":   map[int]string{10: apcIntentEngineer},
			"apcExploreTarget": map[int]apcExploreEntry{10: prevEntry},
			"apcDeliveryProgress": map[int]apcProgressEntry{
				// APC has been at (27,38) since tick 50 → 250 ticks stuck
				// which exceeds apcScoutStallTicks (200).
				10: {LastTick: 50, LastX: 27, LastY: 38},
			},
		},
	}

	if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	targets := getAPCExploreTargets(env.Memory)
	got, ok := targets[10]
	if !ok {
		t.Fatal("expected exploration target after re-pick")
	}
	if got.X == prevEntry.X && got.Y == prevEntry.Y {
		t.Errorf("expected re-pick to exclude stuck waypoint (%d,%d), still got it", prevEntry.X, prevEntry.Y)
	}
}

// APC scout mode must rotate through every waypoint, matching ActionScoutPatrol.
// Previous farthest-first pick oscillated between opposite corners and left
// mid-edges permanently unvisited — so enemy bases along map edges were never
// sighted, HasEnemyIntel() stayed false, and doctrines waiting on building
// intel sat at base forever.
func TestActionUnloadAPCNearTarget_RoundRobinVisitsAllWaypoints(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Map size where generateWaypoints will return 9 distinct candidates.
	mapW, mapH := 128, 128
	waypoints := generateWaypoints(mapW, mapH, nil)
	if len(waypoints) != 9 {
		t.Fatalf("test precondition: expected 9 waypoints, got %d", len(waypoints))
	}

	visited := make(map[[2]int]bool)
	// Simulate the APC teleporting to each assigned waypoint (arrival) so the
	// round-robin advances. Over len(waypoints) iterations every waypoint
	// should be seen at least once.
	apcPos := [2]int{96, 99} // near map corner, like the live game
	mem := map[string]any{"apcCargoIntent": map[int]string{10: apcIntentEngineer}}

	for i := 0; i < len(waypoints)+2; i++ {
		env := RuleEnv{
			State: model.GameState{
				Tick:      1000 + i*10,
				MapWidth:  mapW,
				MapHeight: mapH,
				Units: []model.Unit{
					{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: apcPos[0], Y: apcPos[1]},
				},
			},
			Memory: mem,
		}
		if err := ActionUnloadAPCNearTarget(env, conn); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		entry := getAPCExploreTargets(mem)[10]
		visited[[2]int{entry.X, entry.Y}] = true
		// Simulate arrival so next iteration advances.
		apcPos = [2]int{entry.X, entry.Y}
	}

	for _, wp := range waypoints {
		if !visited[[2]int{wp[0], wp[1]}] {
			t.Errorf("waypoint %v never visited — round-robin broken, coverage incomplete", wp)
		}
	}
}

// Capture commands must be throttled — re-issuing the same Capture order each
// tick cancels the in-flight walk-to-target activity in OpenRA, so engineers
// never arrive. Observed live: 111 captures of the same (engineer,target)
// pair in a single game with zero completions.
func TestActionCaptureBuilding_ThrottlesOrderSpam(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	mkEnv := func(tick int, mem map[string]any) RuleEnv {
		return RuleEnv{
			State: model.GameState{
				Tick: tick,
				Units: []model.Unit{
					{ID: 50, Type: "e6", Idle: true, X: 10, Y: 10},
				},
				Capturables: []model.Enemy{
					{ID: 900, Type: "oilb", Owner: "Neutral", X: 20, Y: 20},
				},
			},
			Memory: mem,
		}
	}

	mem := map[string]any{}
	if err := ActionCaptureBuilding(mkEnv(1000, mem), conn); err != nil {
		t.Fatalf("first call: %v", err)
	}
	state := getCaptureOrderState(mem)
	if state[50].TargetID != 900 || state[50].Tick != 1000 {
		t.Fatalf("expected seed entry {target=900 tick=1000}, got %+v", state[50])
	}

	// Same target inside resend window → suppressed (tick frozen).
	if err := ActionCaptureBuilding(mkEnv(1000+captureOrderResend-1, mem), conn); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if state[50].Tick != 1000 {
		t.Errorf("expected tick to stay at 1000 (suppressed), got %d", state[50].Tick)
	}

	// Past the window → resend (tick advances).
	if err := ActionCaptureBuilding(mkEnv(1000+captureOrderResend+1, mem), conn); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if state[50].Tick != 1000+captureOrderResend+1 {
		t.Errorf("expected tick to advance after window, got %d", state[50].Tick)
	}

	// Different target inside window → resend immediately (target changed).
	envNewTarget := RuleEnv{
		State: model.GameState{
			Tick: 1000 + captureOrderResend + 2,
			Units: []model.Unit{
				{ID: 50, Type: "e6", Idle: true, X: 10, Y: 10},
			},
			Capturables: []model.Enemy{
				{ID: 901, Type: "oilb", Owner: "Neutral", X: 30, Y: 30},
			},
		},
		Memory: mem,
	}
	if err := ActionCaptureBuilding(envNewTarget, conn); err != nil {
		t.Fatalf("fourth call: %v", err)
	}
	if state[50].TargetID != 901 {
		t.Errorf("expected target to flip to 901 on change, got %d", state[50].TargetID)
	}
}
