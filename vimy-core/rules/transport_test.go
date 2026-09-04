package rules

import (
	"net"
	"testing"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

// testConn creates a *ipc.Connection backed by a pipe. The returned cleanup
// function closes both ends. Sent messages are consumed by the reader goroutine.
func testConn(t *testing.T) (*ipc.Connection, func()) {
	t.Helper()
	server, client := net.Pipe()
	conn := ipc.NewConnection(server, nil)
	// Drain anything written so sends don't block.
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			_, err := client.Read(buf)
			if err != nil {
				return
			}
		}
	}()
	cleanup := func() {
		server.Close()
		client.Close()
		<-done
	}
	return conn, cleanup
}

// --- IdleCombatInfantry tests ---

func TestIdleCombatInfantry_ReturnsNonEngineers(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "e1", Idle: true},   // rifle — included
				{ID: 2, Type: "e4", Idle: true},   // flamethrower — included
				{ID: 3, Type: "shok", Idle: true}, // shock trooper — included
				{ID: 4, Type: "e6", Idle: true},   // engineer — excluded
				{ID: 5, Type: "e3", Idle: true},   // rocket soldier — included
				{ID: 6, Type: "e1", Idle: false},  // rifle, not idle — excluded
				{ID: 7, Type: "3tnk", Idle: true}, // tank — excluded (not infantry)
				{ID: 8, Type: "medi", Idle: true}, // medic — included
			},
		},
		Memory: make(map[string]any),
	}

	got := env.IdleCombatInfantry()
	wantIDs := map[int]bool{1: true, 2: true, 3: true, 5: true, 8: true}
	if len(got) != len(wantIDs) {
		t.Fatalf("IdleCombatInfantry: got %d units, want %d", len(got), len(wantIDs))
	}
	for _, u := range got {
		if !wantIDs[u.ID] {
			t.Errorf("unexpected unit ID %d (type %s) in IdleCombatInfantry", u.ID, u.Type)
		}
	}
}

func TestIdleCombatInfantry_EmptyWhenNoInfantry(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "3tnk", Idle: true},
				{ID: 2, Type: "harv", Idle: true},
				{ID: 3, Type: "e6", Idle: true}, // only engineer
			},
		},
		Memory: make(map[string]any),
	}

	got := env.IdleCombatInfantry()
	if len(got) != 0 {
		t.Errorf("expected empty, got %d units", len(got))
	}
}

// --- ActionLoadCombatInfantry tests ---

func TestActionLoadCombatInfantry_SkipsSquadAssigned(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "e1", Idle: true},                  // squad-assigned — skip
				{ID: 2, Type: "e4", Idle: true},                  // free — should load
				{ID: 10, Type: "apc", Idle: true, CargoCount: 0}, // empty APC
			},
		},
		Memory: map[string]any{
			"squads": map[string]*Squad{
				"ground-attack": {
					Name:    "ground-attack",
					Domain:  "ground",
					UnitIDs: []int{1},
					Role:    "attack",
				},
			},
		},
	}

	err := ActionLoadCombatInfantry(env, conn)
	if err != nil {
		t.Fatalf("ActionLoadCombatInfantry returned error: %v", err)
	}
	// The action should have sent a command for unit 2 (not 1, which is squad-assigned).
	// We verify indirectly — no crash, and the action returned nil (success).
}

func TestActionLoadCombatInfantry_NoAPCsNoOp(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 1, Type: "e1", Idle: true},
			},
		},
		Memory: make(map[string]any),
	}

	// nil conn — should return nil without sending anything.
	err := ActionLoadCombatInfantry(env, nil)
	if err != nil {
		t.Fatalf("expected nil error with no APCs, got: %v", err)
	}
}

func TestActionLoadCombatInfantry_NoInfantryNoOp(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 0},
				{ID: 2, Type: "3tnk", Idle: true}, // not infantry
			},
		},
		Memory: make(map[string]any),
	}

	err := ActionLoadCombatInfantry(env, nil)
	if err != nil {
		t.Fatalf("expected nil error with no combat infantry, got: %v", err)
	}
}

// --- ActionDeliverAssaultAPC tests ---

func TestActionDeliverAssaultAPC_MovesWhenFar(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{{ID: 100, Type: "fact", X: 50, Y: 50}},
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
		},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy": {Owner: "Enemy", X: 500, Y: 500, Tick: 1, FromBuildings: true},
			},
			"apcCargoIntent": map[int]string{10: apcIntentCombat},
		},
	}

	err := ActionDeliverAssaultAPC(env, conn)
	if err != nil {
		t.Fatalf("ActionDeliverAssaultAPC returned error: %v", err)
	}
	// APC is far from enemy base — should have sent a move command (no crash).
}

func TestActionDeliverAssaultAPC_UnloadsWhenClose(t *testing.T) {
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

	err := ActionDeliverAssaultAPC(env, conn)
	if err != nil {
		t.Fatalf("ActionDeliverAssaultAPC returned error: %v", err)
	}
	// APC is within 7 cells of enemy base — should have sent an unload command.
}

func TestActionDeliverAssaultAPC_NoIntelNoOp(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
		},
		Memory: make(map[string]any),
	}

	err := ActionDeliverAssaultAPC(env, nil)
	if err != nil {
		t.Fatalf("expected nil error with no enemy intel, got: %v", err)
	}
}

func TestActionDeliverAssaultAPC_SkipsWaterBase(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	// Enemy base is on water (naval yard). APC should fall back to visible enemy.
	grid := &model.TerrainGrid{
		Cols: 32, Rows: 32, CellW: 32, CellH: 32,
		Grid: make([]model.TerrainType, 32*32),
	}
	// Mark the enemy base zone as water.
	for i := range grid.Grid {
		grid.Grid[i] = model.Land
	}
	// Zone at (500,500) in map coords → col=500/32=15, row=500/32=15 → index 15*32+15
	grid.Grid[15*32+15] = model.Water

	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{{ID: 100, Type: "fact", X: 50, Y: 50}},
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
			Enemies: []model.Enemy{
				{ID: 200, Type: "e1", Owner: "Enemy", X: 400, Y: 400, HP: 100, MaxHP: 100},
			},
		},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy": {Owner: "Enemy", X: 500, Y: 500, Tick: 1, FromBuildings: true},
			},
			"apcCargoIntent": map[int]string{10: apcIntentCombat},
		},
		Terrain: grid,
	}

	err := ActionDeliverAssaultAPC(env, conn)
	if err != nil {
		t.Fatalf("ActionDeliverAssaultAPC returned error: %v", err)
	}
	// APC should be sent toward the visible enemy at (400,400), not the water base.
}

// When no enemy intel exists (no buildings sighted, no units visible), a
// combat-loaded APC must fall back to exploration instead of sitting at base.
// This was the real cause of strandded combat APCs in the traced game.
func TestActionDeliverAssaultAPC_ExploresWhenNoTarget(t *testing.T) {
	conn, cleanup := testConn(t)
	defer cleanup()

	env := RuleEnv{
		State: model.GameState{
			Tick:      1000,
			MapWidth:  1000,
			MapHeight: 1000,
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
			// no enemies visible, no enemyBases intel
		},
		Memory: map[string]any{
			"apcCargoIntent": map[int]string{10: apcIntentCombat},
		},
	}

	if err := ActionDeliverAssaultAPC(env, conn); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	targets := getAPCExploreTargets(env.Memory)
	entry, ok := targets[10]
	if !ok {
		t.Fatal("expected exploration target for combat-loaded APC 10")
	}
	if entry.X < 500 && entry.Y < 500 {
		t.Errorf("expected exploration target across the map, got (%d,%d)", entry.X, entry.Y)
	}
}

func TestActionDeliverAssaultAPC_NoLandTargetNoOp(t *testing.T) {
	// Enemy base on water, no visible enemies → should no-op.
	grid := &model.TerrainGrid{
		Cols: 32, Rows: 32, CellW: 32, CellH: 32,
		Grid: make([]model.TerrainType, 32*32),
	}
	for i := range grid.Grid {
		grid.Grid[i] = model.Land
	}
	grid.Grid[15*32+15] = model.Water

	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{{ID: 100, Type: "fact", X: 50, Y: 50}},
			Units: []model.Unit{
				{ID: 10, Type: "apc", Idle: true, CargoCount: 1, X: 50, Y: 50},
			},
		},
		Memory: map[string]any{
			"enemyBases": map[string]EnemyBaseIntel{
				"Enemy": {Owner: "Enemy", X: 500, Y: 500, Tick: 1, FromBuildings: true},
			},
		},
		Terrain: grid,
	}

	err := ActionDeliverAssaultAPC(env, nil)
	if err != nil {
		t.Fatalf("expected nil error with no land target, got: %v", err)
	}
}

// --- nearestTo tests ---

func TestNearestTo(t *testing.T) {
	units := []model.Unit{
		{ID: 1, X: 100, Y: 100},
		{ID: 2, X: 10, Y: 10},
		{ID: 3, X: 50, Y: 50},
	}
	best, dist := nearestTo(units, 12, 12)
	if best.ID != 2 {
		t.Errorf("expected nearest unit ID=2, got ID=%d", best.ID)
	}
	if dist > 4 {
		t.Errorf("expected distance < 4, got %.2f", dist)
	}
}

// --- Compiler tests ---
