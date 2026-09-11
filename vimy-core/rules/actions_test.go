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
	// One building clamps the radius to 3, so every candidate is near it.
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
	// A refinery far from the main cluster. Normally threat-toward-enemy wins;
	// with harvesters fleeing, placement should shift to the refinery.
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
	// The bridge at zone (3,2) is the only crossing to the enemy side, so
	// defenses should favour it over the far side of the base.
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

	// Never on the bridge itself: that strands our own units.
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

// The unload threshold has to survive APC pathing stalls; too tight and the
// engineer never disembarks.
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

// Past apcStallTicks the APC unloads regardless of distance — stall is the
// fallback path.
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
				// A generous gap, so a raised threshold doesn't invalidate this.
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

// A brief non-movement window — the server not having started pathing — must not
// read as a stall, or APCs unload at base and never reach the target.
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

// Re-issuing the same destination cancels the in-flight path and pins the APC,
// so Move must be throttled.
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

// With nothing to capture the loaded APC scouts rather than idling — what
// aggressive-capture doctrines depend on.
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

// The waypoint index must survive re-invocation, or the APC picks a fresh
// destination every tick and reaches none. The index is authoritative; X,Y are
// derived from it.
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

// The core invariant for split-intent delivery: neither list picks up the
// other's APCs.
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

// Dead APCs are pruned; live empty ones keep their tag. A freshly-loaded APC
// reads as empty for a tick or two, and pruning there strands it — no deliver
// rule picks it up once the cargo finally registers.
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

// Unload clears the tag, so the APC can be re-tagged for a new mission.
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

func TestDesignateScout_PrefersDogOverLightTank(t *testing.T) {
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
	if getScoutID(env.Memory) != 20 {
		t.Errorf("expected attack dog (id=20) designated as scout, got %d", getScoutID(env.Memory))
	}
}

// With no dog, a light tank is worth scouting with only once the armour can
// spare it. Game 102 peaked at three combat vehicles and put its only light
// tank on patrol, where it drove into the enemy base and died.
func TestDesignateScout_LightTankOnlyWhenArmourCanSpareIt(t *testing.T) {
	lightTankScout := func(units []model.Unit) int {
		env := RuleEnv{State: model.GameState{Units: units}, Memory: make(map[string]any)}
		designateScout(env)
		return getScoutID(env.Memory)
	}

	scarce := []model.Unit{
		{ID: 21, Type: "1tnk", Idle: true},
		{ID: 22, Type: "2tnk", Idle: true},
		{ID: 23, Type: "arty", Idle: true},
	}
	if got := lightTankScout(scarce); got != 0 {
		t.Errorf("three combat vehicles: expected no scout designated, got %d", got)
	}

	spare := append(append([]model.Unit{}, scarce...), model.Unit{ID: 24, Type: "2tnk", Idle: true})
	if got := lightTankScout(spare); got != 21 {
		t.Errorf("four combat vehicles: expected light tank (id=21) designated, got %d", got)
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

// A scouting APC stuck on an unreachable waypoint must re-pick. A deterministic
// pick returns the same blocked corner every TTL window and the APC never moves.
func TestActionUnloadAPCNearTarget_StalledScoutRepicksDifferentWaypoint(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Enough map for several candidates, with a stale assignment to a far corner.
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
				// 250 ticks stuck, past apcScoutStallTicks.
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

// Scout mode rotates through every waypoint, as ActionScoutPatrol does.
// Farthest-first oscillates between opposite corners and never visits mid-edges,
// so a map-edge enemy base is never sighted and intel-gated doctrines never move.
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
	// Teleport the APC to each assignment so the rotation advances; every
	// waypoint should come up within one full cycle.
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
		apcPos = [2]int{entry.X, entry.Y}
	}

	for _, wp := range waypoints {
		if !visited[[2]int{wp[0], wp[1]}] {
			t.Errorf("waypoint %v never visited — round-robin broken, coverage incomplete", wp)
		}
	}
}

// Re-issuing a Capture cancels the in-flight walk to the target, so engineers
// never arrive without throttling.
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

// The return rule leaves alone whatever the flee rule is moving.
//
// Both fire on the same tick in different categories, so exclusivity cannot
// arbitrate, and both aim at the nearest refinery — the harvester arrives idle,
// is sent back out, and walks straight into danger again.
//
// Asserted on the `harvestSent` entries rather than the wire: they are what the
// resend guard itself reads.
// longIdle marks every harvester as having been idle past the grace period, so
// a test about the flee interaction is not also testing the grace period. The
// rule leaves a freshly idle harvester to the engine's own ore search.
func longIdle(env RuleEnv) {
	since := memoryMap[int, int](env.Memory, "harvesterIdleSince")
	for _, u := range env.State.Units {
		since[u.ID] = env.State.Tick - harvesterIdleGrace - 1
	}
}

func TestSendIdleHarvestersSkipsFleeingOnes(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	newEnv := func() RuleEnv {
		return RuleEnv{
			State: model.GameState{
				Tick:      5000,
				Units:     []model.Unit{{ID: 1, Type: "harv", Idle: true}, {ID: 2, Type: "harv", Idle: true}},
				Buildings: []model.Building{{ID: 10, Type: "proc", X: 20, Y: 20}},
			},
			Memory: map[string]any{},
		}
	}

	env := newEnv()
	longIdle(env)
	if err := ActionSendIdleHarvesters(env, conn); err != nil {
		t.Fatal(err)
	}
	sent := memoryMap[int, harvestEntry](env.Memory, "harvestSent")
	if len(sent) != 2 {
		t.Fatalf("dispatched %d harvesters, want both", len(sent))
	}

	// Now one of them is mid-flee. It must be left alone; the other still goes.
	env = newEnv()
	longIdle(env)
	getHarvesterFleeState(env.Memory)[1] = harvesterFleeEntry{Tick: 4990, X: 20, Y: 20}
	if err := ActionSendIdleHarvesters(env, conn); err != nil {
		t.Fatal(err)
	}
	sent = memoryMap[int, harvestEntry](env.Memory, "harvestSent")
	if len(sent) != 1 {
		t.Fatalf("dispatched %d harvesters, want only the one that is not fleeing", len(sent))
	}
	if _, ok := sent[2]; !ok {
		t.Errorf("dispatched the fleeing harvester instead of the free one: %+v", sent)
	}

	// The map is pruned only while something is in danger, so a stale entry can
	// sit untouched — and must not hold the harvester forever.
	env = newEnv()
	longIdle(env)
	getHarvesterFleeState(env.Memory)[1] = harvesterFleeEntry{
		Tick: env.State.Tick - harvesterFleeResend - 1, X: 20, Y: 20,
	}
	if err := ActionSendIdleHarvesters(env, conn); err != nil {
		t.Fatal(err)
	}
	sent = memoryMap[int, harvestEntry](env.Memory, "harvestSent")
	if len(sent) != 2 {
		t.Fatalf("a stale flee entry still froze a harvester: dispatched %d", len(sent))
	}
}

// With nothing in danger the flee map empties, rather than keeping entries that
// nothing will ever remove.
func TestFleeHarvestersClearsItsMapWhenTheThreatLeaves(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Tick: 5000, MapWidth: 100, MapHeight: 100,
			Units:     []model.Unit{{ID: 1, Type: "harv", X: 50, Y: 50, HP: 600, MaxHP: 600}},
			Buildings: []model.Building{{ID: 10, Type: "proc", X: 20, Y: 20}},
			// No enemies, so nothing is in danger.
		},
		Memory: map[string]any{},
	}
	getHarvesterFleeState(env.Memory)[1] = harvesterFleeEntry{Tick: 4990, X: 20, Y: 20}

	if err := FleeHarvesters(0.1)(env, conn); err != nil {
		t.Fatal(err)
	}
	if n := len(getHarvesterFleeState(env.Memory)); n != 0 {
		t.Errorf("%d stale entries left behind", n)
	}
}

// Successive relocations must pick different tiles. A fallback derived purely
// from the building centroid returns the same one forever, and each is followed
// by three more deploys at a spot that has already failed three times.
func TestMCVFallbackTriesSomewhereNew(t *testing.T) {
	all := make([]model.TerrainType, 64)
	for i := range all {
		all[i] = model.Land
	}
	env := RuleEnv{
		State:   model.GameState{MapWidth: 800, MapHeight: 800},
		Terrain: &model.TerrainGrid{Cols: 8, Rows: 8, CellW: 100, CellH: 100, Grid: all},
	}
	mcv := &model.Unit{ID: 1, Type: MCV, X: 400, Y: 400}

	seen := map[[2]int]int{}
	for round := 0; round < 6; round++ {
		x, y := mcvFallbackLocation(env, mcv, round)
		seen[[2]int{x, y}]++
	}
	if len(seen) < 4 {
		t.Errorf("six relocations produced %d distinct tiles (%v); the loop is still retrying the same spot", len(seen), seen)
	}
}

// Anchored on the MCV, not the surviving base. A centroid of remaining buildings
// converges on the fighting while a base is overrun — the worst place to deploy,
// and the only situation this runs in.
func TestMCVFallbackIgnoresTheShrinkingBase(t *testing.T) {
	all := make([]model.TerrainType, 64)
	for i := range all {
		all[i] = model.Land
	}
	env := RuleEnv{
		State: model.GameState{
			MapWidth: 800, MapHeight: 800,
			Buildings: []model.Building{{ID: 1, Type: "powr", X: 50, Y: 50}},
		},
		Terrain: &model.TerrainGrid{Cols: 8, Rows: 8, CellW: 100, CellH: 100, Grid: all},
	}
	far := &model.Unit{ID: 1, Type: MCV, X: 700, Y: 700}
	x, y := mcvFallbackLocation(env, far, 0)
	if x < 300 || y < 300 {
		t.Errorf("fallback (%d,%d) drifted toward the building at (50,50); it should stay near the MCV at (700,700)", x, y)
	}
}

// Ore is not in the game state, so this rule can only say "mine near a
// refinery". When that patch is exhausted the harvester finds nothing, goes
// idle, and is ordered back — game 96 matched 746 times while the harvesters
// sat in a heap beside the refineries. The engine knows where ore is and we do
// not, so it gets first refusal: an order issued the instant a harvester goes
// idle interrupts its own search.
func TestIdleHarvestersAreLeftToTheEngineBriefly(t *testing.T) {
	memory := map[string]any{}
	env := func(tick int) RuleEnv {
		return RuleEnv{
			Memory: memory,
			State: model.GameState{
				Tick:      tick,
				Units:     []model.Unit{{ID: 1, Type: "harv", X: 500, Y: 500, Idle: true}},
				Buildings: []model.Building{{ID: 9, Type: "proc", X: 400, Y: 400}},
			},
		}
	}
	conn, cleanup := testConn(t)
	defer cleanup()

	before := conn.Sent()
	if err := ActionSendIdleHarvesters(env(1000), conn); err != nil {
		t.Fatal(err)
	}
	if conn.Sent() != before {
		t.Error("ordered a harvester the moment it went idle, interrupting the engine's ore search")
	}
	// Still idle much later: now it is genuinely stuck and worth a nudge.
	if err := ActionSendIdleHarvesters(env(1000+harvesterIdleGrace+1), conn); err != nil {
		t.Fatal(err)
	}
	if conn.Sent() == before {
		t.Error("a harvester idle well past the grace period was never rescued")
	}
}

// The grace period exists for a harvester whose idleness we cannot explain, so
// the engine gets first go at finding ore. One that has just fled is idle for a
// reason we already know — it ran to a refinery and stopped — and making it
// wait is lost mining. Game 97 fled 116 times.
func TestFledHarvestersSkipTheGracePeriod(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		Memory: map[string]any{},
		State: model.GameState{
			Tick:      5000,
			Units:     []model.Unit{{ID: 1, Type: "harv", Idle: true}},
			Buildings: []model.Building{{ID: 10, Type: "proc", X: 20, Y: 20}},
		},
	}
	// It fled a while ago — past the flee-resend window, so it is no longer
	// mid-move, just sitting there.
	getHarvesterFleeState(env.Memory)[1] = harvesterFleeEntry{
		Tick: env.State.Tick - harvesterFleeResend - 1, X: 20, Y: 20,
	}
	if err := ActionSendIdleHarvesters(env, conn); err != nil {
		t.Fatal(err)
	}
	if sent := memoryMap[int, harvestEntry](env.Memory, "harvestSent"); len(sent) != 1 {
		t.Errorf("a harvester that fled and stopped was left idle for the grace period: %+v", sent)
	}
}

// The base hunt has to be able to reach a building that outlived the base it
// was part of. Game 105 reduced the enemy to one outlying barracks and then
// circled the empty base site: two rings at four cells could not span a 128
// cell map, and the squad wandered until it happened to see the building.
func TestHuntReachesAcrossTheMap(t *testing.T) {
	const mapDim = 128

	for _, tc := range []struct {
		name       string
		aggression float64
	}{
		{"cautious", 0.25},
		{"aggressive", 0.8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			radius := int(float64(mapDim/16) * (0.25 + tc.aggression*1.25))
			if radius < 1 {
				radius = 1
			}
			maxStep := huntMaxStep(mapDim, radius)

			var reach int
			for step := 1; step <= maxStep; step++ {
				dx, dy := huntOffset(step, radius)
				if d := max(abs(dx), abs(dy)); d > reach {
					reach = d
				}
			}
			// Half the map: the enemy start is across it, not beside us.
			if want := mapDim / 2; reach < want {
				t.Errorf("aggression %.2f: hunt reaches %d cells, want at least %d", tc.aggression, reach, want)
			}
		})
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// The scout party has to be the SAME two units each firing. A scout only
// advances to its next waypoint when this action runs for it, so a unit that
// loses its place is a unit left standing wherever it happened to be — six
// firings early in game 105 parked units in corners across the map.
func TestScoutWithIdleUnitsKeepsTheSameParty(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	grid := &model.TerrainGrid{Cols: 4, Rows: 4, CellW: 100, CellH: 100, Grid: make([]model.TerrainType, 16)}
	for i := range grid.Grid {
		grid.Grid[i] = model.Land
	}

	mem := map[string]any{}
	env := func(units []model.Unit, tick int) RuleEnv {
		return RuleEnv{
			State:   model.GameState{Tick: tick, MapWidth: 400, MapHeight: 400, Units: units},
			Memory:  mem,
			Terrain: grid,
		}
	}

	first := []model.Unit{{ID: 1, Type: "e1", Idle: true}, {ID: 2, Type: "e1", Idle: true}}
	if err := ActionScoutWithIdleUnits(env(first, 100), conn); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	sent := memoryMap[int, scoutMoveEntry](mem, "idleScoutMoveSent")
	if len(sent) != scoutPartySize {
		t.Fatalf("expected %d scouts assigned, got %d", scoutPartySize, len(sent))
	}

	// Newly built rifles arrive ahead of the scouts in State.Units order.
	withNew := []model.Unit{
		{ID: 3, Type: "e1", Idle: true},
		{ID: 4, Type: "e1", Idle: true},
		{ID: 1, Type: "e1", Idle: true},
		{ID: 2, Type: "e1", Idle: true},
	}
	if err := ActionScoutWithIdleUnits(env(withNew, 1000), conn); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}

	sent = memoryMap[int, scoutMoveEntry](mem, "idleScoutMoveSent")
	if len(sent) != scoutPartySize {
		t.Errorf("party grew to %d units; the displaced ones strand where they stand", len(sent))
	}
	for _, id := range []int{1, 2} {
		if _, ok := sent[id]; !ok {
			t.Errorf("unit %d lost its place to a newly built unit", id)
		}
	}
}

// Repair is gated on the money the rules actually spend. Read against
// Player.Cash alone the floor was unreachable — ore sits in Resources until it
// converts — and repair acted zero times in every recorded game while
// buildings burned.
func TestRepairSpendsAgainstTotalCash(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	damaged := []model.Building{{ID: 1, Type: "proc", HP: 100, MaxHP: 1000, X: 10, Y: 10}}

	repaired := func(cash, resources int) bool {
		env := RuleEnv{
			State: model.GameState{
				Tick:      1000,
				Buildings: damaged,
				Player:    model.Player{Cash: cash, Resources: resources},
			},
			Memory: map[string]any{},
		}
		if err := ActionRepairDamagedBuildings(env, conn); err != nil {
			t.Fatalf("repair: %v", err)
		}
		return len(memoryMap[int, repairToggleEntry](env.Memory, "repairToggleSent")) > 0
	}

	// The floor's worth, held mostly as unconverted ore. This is the ordinary
	// case that never repaired.
	if !repaired(100, 600) {
		t.Error("cash 100 + resources 600 is above the floor, but nothing was repaired")
	}
	// Genuinely broke: production and rebuild need what is left.
	if repaired(100, 100) {
		t.Error("cash 100 + resources 100 is below the floor, but repair started anyway")
	}
}
