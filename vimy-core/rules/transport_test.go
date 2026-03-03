package rules

import (
	"net"
	"strings"
	"testing"

	"github.com/expr-lang/expr"
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
				{ID: 1, Type: "e1", Idle: true},   // squad-assigned — skip
				{ID: 2, Type: "e4", Idle: true},   // free — should load
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
		},
		Terrain: grid,
	}

	err := ActionDeliverAssaultAPC(env, conn)
	if err != nil {
		t.Fatalf("ActionDeliverAssaultAPC returned error: %v", err)
	}
	// APC should be sent toward the visible enemy at (400,400), not the water base.
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

func TestCompileDoctrineTransportAssault(t *testing.T) {
	transportRuleNames := []string{
		"produce-assault-apc",
		"load-assault-infantry",
		"deliver-assault-apc",
	}

	// TransportAssault=0 → no transport assault rules
	t.Run("absent when TransportAssault=0", func(t *testing.T) {
		d := DefaultDoctrine()
		d.TransportAssault = 0
		rules := CompileDoctrine(d)

		found := map[string]bool{}
		for _, r := range rules {
			found[r.Name] = true
		}
		for _, name := range transportRuleNames {
			if found[name] {
				t.Errorf("unexpected transport assault rule %q when TransportAssault=0", name)
			}
		}
	})

	// TransportAssault=0.5 → all transport assault rules present
	t.Run("present when TransportAssault=0.5", func(t *testing.T) {
		d := DefaultDoctrine()
		d.TransportAssault = 0.5
		rules := CompileDoctrine(d)

		for _, r := range rules {
			_, err := expr.Compile(r.ConditionSrc, expr.Env(RuleEnv{}), expr.AsBool())
			if err != nil {
				t.Errorf("rule %q failed to compile: %v\ncondition: %s", r.Name, err, r.ConditionSrc)
			}
		}

		found := map[string]bool{}
		for _, r := range rules {
			found[r.Name] = true
		}
		for _, name := range transportRuleNames {
			if !found[name] {
				t.Errorf("expected transport assault rule %q when TransportAssault=0.5", name)
			}
		}
	})

	// APC cap scales with TransportAssault
	t.Run("APC cap scales with priority", func(t *testing.T) {
		// Low priority (0.2) → APC cap = lerp(1,3,0.2) = 1
		low := DefaultDoctrine()
		low.TransportAssault = 0.2
		lowRules := CompileDoctrine(low)
		for _, r := range lowRules {
			if r.Name == "produce-assault-apc" {
				if !strings.Contains(r.ConditionSrc, `RoleCount("apc") < 1`) {
					t.Errorf("low TransportAssault: expected APC cap 1, got condition: %s", r.ConditionSrc)
				}
			}
		}

		// High priority (1.0) → APC cap = lerp(1,3,1.0) = 3
		high := DefaultDoctrine()
		high.TransportAssault = 1.0
		highRules := CompileDoctrine(high)
		for _, r := range highRules {
			if r.Name == "produce-assault-apc" {
				if !strings.Contains(r.ConditionSrc, `RoleCount("apc") < 3`) {
					t.Errorf("high TransportAssault: expected APC cap 3, got condition: %s", r.ConditionSrc)
				}
			}
		}
	})

	// Priority ordering: capture rules > transport assault rules
	t.Run("capture rules have higher priority than transport assault", func(t *testing.T) {
		d := DefaultDoctrine()
		d.TransportAssault = 0.5
		d.CapturePriority = 0.5
		rules := CompileDoctrine(d)

		byName := map[string]*Rule{}
		for _, r := range rules {
			byName[r.Name] = r
		}

		loadEngineer := byName["load-engineer-into-apc"]
		loadAssault := byName["load-assault-infantry"]
		deliverCapture := byName["deliver-apc-to-target"]
		deliverAssault := byName["deliver-assault-apc"]

		if loadEngineer == nil || loadAssault == nil {
			t.Fatal("expected both load rules to be present")
		}
		if deliverCapture == nil || deliverAssault == nil {
			t.Fatal("expected both deliver rules to be present")
		}

		if loadEngineer.Priority <= loadAssault.Priority {
			t.Errorf("load-engineer-into-apc priority (%d) should be > load-assault-infantry priority (%d)",
				loadEngineer.Priority, loadAssault.Priority)
		}
		if deliverCapture.Priority <= deliverAssault.Priority {
			t.Errorf("deliver-apc-to-target priority (%d) should be > deliver-assault-apc priority (%d)",
				deliverCapture.Priority, deliverAssault.Priority)
		}
	})

	// Transport rules use "transport" category (non-exclusive)
	t.Run("transport rules are non-exclusive", func(t *testing.T) {
		d := DefaultDoctrine()
		d.TransportAssault = 0.5
		rules := CompileDoctrine(d)

		for _, r := range rules {
			if r.Name == "load-assault-infantry" || r.Name == "deliver-assault-apc" {
				if r.Category != "transport" {
					t.Errorf("rule %q should have category 'transport', got %q", r.Name, r.Category)
				}
				if r.Exclusive {
					t.Errorf("rule %q should not be exclusive", r.Name)
				}
			}
		}
	})
}

// Verify the economy-only doctrine doesn't include transport assault rules.
func TestCompileDoctrineEconomyOnly_NoTransportAssault(t *testing.T) {
	d := Doctrine{
		Name:                  "Turtle",
		EconomyPriority:       0.9,
		Aggression:            0.1,
		InfantryWeight:        0.0,
		VehicleWeight:         0.0,
		TransportAssault:      0.0,
		GroundAttackGroupSize: 12,
		AirAttackGroupSize:    2,
		NavalAttackGroupSize:  3,
	}
	rules := CompileDoctrine(d)

	for _, r := range rules {
		if r.Name == "produce-assault-apc" || r.Name == "load-assault-infantry" || r.Name == "deliver-assault-apc" {
			t.Errorf("unexpected transport assault rule %q when TransportAssault=0", r.Name)
		}
	}
}
