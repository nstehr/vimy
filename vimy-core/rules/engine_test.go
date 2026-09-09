package rules

import (
	"testing"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
)

func TestDefaultRulesCompile(t *testing.T) {
	engine, err := NewEngine(DefaultRules())
	if err != nil {
		t.Fatalf("NewEngine(DefaultRules()) failed: %v", err)
	}
	if len(engine.rules) != 13 {
		t.Errorf("expected 13 rules, got %d", len(engine.rules))
	}
	// Verify priority ordering (descending).
	for i := 1; i < len(engine.rules); i++ {
		if engine.rules[i].Priority > engine.rules[i-1].Priority {
			t.Errorf("rules not sorted by priority: %s (%d) > %s (%d)",
				engine.rules[i].Name, engine.rules[i].Priority,
				engine.rules[i-1].Name, engine.rules[i-1].Priority)
		}
	}
}

func TestFlushFiringStats(t *testing.T) {
	engine, err := NewEngine(DefaultRules())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	engine.SetTraceFirings(true)

	engine.recordFiring("a", 10, true)
	engine.recordFiring("a", 20, false)
	engine.recordFiring("b", 15, false)
	engine.recordFiring("a", 30, true)

	stats := engine.FlushFiringStats()
	if got := stats["a"].FireCount; got != 3 {
		t.Errorf("a.FireCount = %d, want 3", got)
	}
	if got := stats["a"].FirstTick; got != 10 {
		t.Errorf("a.FirstTick = %d, want 10", got)
	}
	if got := stats["a"].LastTick; got != 30 {
		t.Errorf("a.LastTick = %d, want 30", got)
	}
	// A rule that matched three times and acted twice: the gap is the point of
	// keeping both numbers.
	if got := stats["a"].ActCount; got != 2 {
		t.Errorf("a.ActCount = %d, want 2", got)
	}
	if got := stats["b"].ActCount; got != 0 {
		t.Errorf("b.ActCount = %d, want 0 — it matched but never acted", got)
	}
	if got := stats["b"].FireCount; got != 1 {
		t.Errorf("b.FireCount = %d, want 1", got)
	}

	// Flush must reset counters — a second flush with no intervening firings returns empty.
	second := engine.FlushFiringStats()
	if len(second) != 0 {
		t.Errorf("second flush not empty: %+v", second)
	}
}

func TestRuleNamesReturnsCompiledRules(t *testing.T) {
	engine, err := NewEngine(DefaultRules())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	names := engine.RuleNames()
	if len(names) != len(engine.rules) {
		t.Errorf("RuleNames len = %d, want %d", len(names), len(engine.rules))
	}
}

func TestRecordFiringNoOpWhenTracingDisabled(t *testing.T) {
	engine, err := NewEngine(DefaultRules())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	// traceFirings defaults to false — no SetTraceFirings call.
	engine.recordFiring("a", 10, true)
	engine.recordFiring("b", 20, true)
	if stats := engine.FlushFiringStats(); len(stats) != 0 {
		t.Errorf("expected empty stats with tracing off, got %+v", stats)
	}
}

func TestContainsType(t *testing.T) {
	buildings := []model.Building{
		{Type: "powr"},
		{Type: "Tent"},
		{Type: "proc"},
	}

	if !containsType(buildings, "tent") {
		t.Error("containsType should match case-insensitively: tent vs Tent")
	}
	if !containsType(buildings, "POWR") {
		t.Error("containsType should match case-insensitively: POWR vs powr")
	}
	if containsType(buildings, "barr") {
		t.Error("containsType should return false for missing type")
	}

	units := []model.Unit{
		{Type: "e1"},
		{Type: "E1"},
		{Type: "harv"},
	}

	if countType(units, "e1") != 2 {
		t.Errorf("countType(e1) = %d, want 2", countType(units, "e1"))
	}
	if countType(units, "mcv") != 0 {
		t.Errorf("countType(mcv) = %d, want 0", countType(units, "mcv"))
	}
}

func TestFactionVariantMatching(t *testing.T) {
	// Buildings with faction suffixes (e.g. Ukraine faction).
	buildings := []model.Building{
		{Type: "afld.ukraine"},
		{Type: "powr"},
	}

	if !containsType(buildings, "afld") {
		t.Error("containsType should match faction variant afld.ukraine against afld")
	}
	if countType(buildings, "afld") != 1 {
		t.Errorf("countType(afld) = %d, want 1", countType(buildings, "afld"))
	}

	// CanBuildRole with faction variant in buildable list.
	env := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr", "afld.ukraine", "proc"},
				},
			},
		},
	}
	if !env.CanBuildRole("airfield") {
		t.Error("CanBuildRole(airfield) should match afld.ukraine in buildable")
	}

	// BuildableType should return the actual faction variant name.
	got := env.BuildableType("airfield")
	if got != "afld.ukraine" {
		t.Errorf("BuildableType(airfield) = %q, want %q", got, "afld.ukraine")
	}

	// CanBuild with faction variant.
	if !env.CanBuild("Building", "afld") {
		t.Error("CanBuild(Building, afld) should match afld.ukraine in buildable")
	}

	// HasRole with faction variant building.
	envRole := RuleEnv{
		State: model.GameState{
			Buildings: buildings,
		},
	}
	if !envRole.HasRole("airfield") {
		t.Error("HasRole(airfield) should match afld.ukraine building")
	}
	if envRole.RoleCount("airfield") != 1 {
		t.Errorf("RoleCount(airfield) = %d, want 1", envRole.RoleCount("airfield"))
	}
}

func TestHasRole(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{Type: "powr"},
				{Type: "barr"},
			},
			Units: []model.Unit{
				{Type: "e1"},
			},
		},
	}

	if !env.HasRole("barracks") {
		t.Error("HasRole(barracks) should be true with barr building")
	}
	if !env.HasRole("power_plant") {
		t.Error("HasRole(power_plant) should be true with powr building")
	}
	if env.HasRole("war_factory") {
		t.Error("HasRole(war_factory) should be false without weap building")
	}
	if env.HasRole("nonexistent") {
		t.Error("HasRole(nonexistent) should be false for unknown role")
	}

	// Test with Allied barracks (tent).
	envAllied := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{Type: "tent"},
			},
		},
	}
	if !envAllied.HasRole("barracks") {
		t.Error("HasRole(barracks) should be true with tent building")
	}
}

func TestRoleCount(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			Buildings: []model.Building{
				{Type: "powr"},
				{Type: "powr"},
				{Type: "barr"},
			},
		},
	}

	if got := env.RoleCount("power_plant"); got != 2 {
		t.Errorf("RoleCount(power_plant) = %d, want 2", got)
	}
	if got := env.RoleCount("barracks"); got != 1 {
		t.Errorf("RoleCount(barracks) = %d, want 1", got)
	}
}

func TestCanBuildRole(t *testing.T) {
	env := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr", "barr", "proc"},
				},
			},
		},
	}

	if !env.CanBuildRole("barracks") {
		t.Error("CanBuildRole(barracks) should be true with barr in buildable")
	}
	if !env.CanBuildRole("power_plant") {
		t.Error("CanBuildRole(power_plant) should be true with powr in buildable")
	}
	if env.CanBuildRole("war_factory") {
		t.Error("CanBuildRole(war_factory) should be false without weap in buildable")
	}

	// Allied buildable list.
	envAllied := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr", "tent"},
				},
			},
		},
	}
	if !envAllied.CanBuildRole("barracks") {
		t.Error("CanBuildRole(barracks) should be true with tent in buildable")
	}
}

func TestBuildableType(t *testing.T) {
	// Soviet buildable: barr is available but tent is not.
	envSoviet := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr", "barr", "proc"},
				},
			},
		},
	}

	if got := envSoviet.BuildableType("barracks"); got != "barr" {
		t.Errorf("BuildableType(barracks) = %q, want %q", got, "barr")
	}

	// Allied buildable: tent is available but barr is not.
	envAllied := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr", "tent", "proc"},
				},
			},
		},
	}

	if got := envAllied.BuildableType("barracks"); got != "tent" {
		t.Errorf("BuildableType(barracks) = %q, want %q", got, "tent")
	}

	// Neither available.
	envNone := RuleEnv{
		State: model.GameState{
			ProductionQueues: []model.ProductionQueue{
				{
					Type:      "Building",
					Buildable: []string{"powr"},
				},
			},
		},
	}

	if got := envNone.BuildableType("barracks"); got != "" {
		t.Errorf("BuildableType(barracks) = %q, want %q", got, "")
	}

	// Unknown role.
	if got := envSoviet.BuildableType("nonexistent"); got != "" {
		t.Errorf("BuildableType(nonexistent) = %q, want %q", got, "")
	}
}

// A rule whose action returns without doing anything is counted as a match, not
// as work.
//
// This is the distinction the counters exist for: `form-ground-attack` matched
// 15,702 times across thirteen archived losses while forming almost nothing,
// and with only FireCount to look at it read as the busiest rule in the game.
func TestEvaluateSeparatesMatchingFromActing(t *testing.T) {
	worked := &Rule{
		Name:         "does-something",
		Priority:     100,
		Category:     "test",
		ConditionSrc: "true",
		Action: func(env RuleEnv, conn *ipc.Connection) error {
			markEffect(env)
			return nil
		},
	}
	// The shape of every action that bails on a precondition its rule does not
	// mirror.
	idle := &Rule{
		Name:         "does-nothing",
		Priority:     90,
		Category:     "test2",
		ConditionSrc: "true",
		Action:       func(env RuleEnv, conn *ipc.Connection) error { return nil },
	}

	engine, err := NewEngine([]*Rule{worked, idle})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	engine.SetTraceFirings(true)

	// nil conn: no orders can be sent, so markEffect is the only signal, which
	// is exactly the case the order counter cannot cover.
	for range 3 {
		if err := engine.Evaluate(model.GameState{Tick: 1}, "england", nil); err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
	}

	stats := engine.FlushFiringStats()
	if got := stats["does-something"].FireCount; got != 3 {
		t.Errorf("does-something.FireCount = %d, want 3", got)
	}
	if got := stats["does-something"].ActCount; got != 3 {
		t.Errorf("does-something.ActCount = %d, want 3", got)
	}
	if got := stats["does-nothing"].FireCount; got != 3 {
		t.Errorf("does-nothing.FireCount = %d, want 3", got)
	}
	if got := stats["does-nothing"].ActCount; got != 0 {
		t.Errorf("does-nothing.ActCount = %d, want 0 — it matched but never acted", got)
	}
}

// squadRule builds a rule whose action is a form-squad call, the way an
// artifact loaded from vimyc carries it.
func squadRule(name, squad string) *Rule {
	return &Rule{
		Name:         name,
		Priority:     100,
		Category:     "squad-form",
		ConditionSrc: "true",
		ActionSrc:    "form-squad(" + squad + ", Ground, 4, Attack)",
		Action:       FormSquad(squad, "ground", 4, "attack"),
	}
}

func seedSquad(e *Engine, name string, ids ...int) {
	squads := getSquads(e.Memory)
	squads[name] = &Squad{Name: name, Domain: "ground", UnitIDs: ids, TargetSize: 4}
	e.Memory["squads"] = squads
}

// A doctrine swap keeps the squads the new rules still form.
//
// The strategist swaps every twenty seconds or so; dropping every squad each
// time meant none ever lived long enough to reach commit strength.
func TestSwapKeepsSquadsTheNewRulesStillForm(t *testing.T) {
	engine, err := NewEngine([]*Rule{squadRule("form-ground-attack", "ground-attack")})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	seedSquad(engine, "ground-attack", 1, 2, 3)

	if err := engine.Swap([]*Rule{squadRule("form-ground-attack", "ground-attack")}); err != nil {
		t.Fatalf("Swap: %v", err)
	}

	sq, ok := getSquads(engine.Memory)["ground-attack"]
	if !ok {
		t.Fatal("ground-attack was dropped by a swap that still forms it")
	}
	if len(sq.UnitIDs) != 3 {
		t.Errorf("members = %d, want the original 3", len(sq.UnitIDs))
	}
}

// A squad the new rules cannot form is dropped: nothing would reinforce or
// command it, and its members would stay assigned and invisible to the free
// pool forever.
func TestSwapDropsSquadsTheNewRulesCannotForm(t *testing.T) {
	engine, err := NewEngine([]*Rule{squadRule("form-harvester-guard", "harvester-guard")})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	seedSquad(engine, "harvester-guard", 1, 2)
	seedSquad(engine, "ground-attack", 3, 4)
	engine.Memory["huntBase:ground-attack"] = "somewhere"

	if err := engine.Swap([]*Rule{squadRule("form-harvester-guard", "harvester-guard")}); err != nil {
		t.Fatalf("Swap: %v", err)
	}

	squads := getSquads(engine.Memory)
	if _, ok := squads["ground-attack"]; ok {
		t.Error("ground-attack survived a swap to rules that cannot form it")
	}
	if _, ok := squads["harvester-guard"]; !ok {
		t.Error("harvester-guard was dropped by a swap that still forms it")
	}
	if _, ok := engine.Memory["huntBase:ground-attack"]; ok {
		t.Error("the dropped squad left its hunt state behind")
	}
}

// A rule set that forms no squads at all — the seed set, or an artifact with no
// ActionSrc — drops everything, which is the old behaviour and the safe one.
func TestSwapToRulesWithoutSquadsDropsThemAll(t *testing.T) {
	engine, err := NewEngine([]*Rule{squadRule("form-ground-attack", "ground-attack")})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	seedSquad(engine, "ground-attack", 1, 2)

	plain := &Rule{
		Name: "repair-buildings", Priority: 10, Category: "maintenance",
		ConditionSrc: "true", Action: ActionRepairDamagedBuildings,
	}
	if err := engine.Swap([]*Rule{plain}); err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if len(getSquads(engine.Memory)) != 0 {
		t.Error("squads survived a swap to a rule set that forms none")
	}
}

// income-rate must be measured on the same money the rules spend. Player.Cash
// is the spendable half; ore waiting in the silos is Player.Resources, and
// RuleEnv.Cash sums both. Sampling the spendable half alone made income-rate
// non-positive in every state of game 89, which armed the savings model
// permanently and starved base defences.
func TestSampleIncomeCountsOreInSilos(t *testing.T) {
	mem := map[string]any{}
	env := func(tick, cash, res int) RuleEnv {
		return RuleEnv{Memory: mem, State: model.GameState{
			Tick:   tick,
			Player: model.Player{Cash: cash, Resources: res},
		}}
	}
	// First call only seeds the baseline.
	sampleIncome(env(0, 0, 1000))
	if _, ok := mem["incomeRate"]; ok {
		t.Fatal("the first sample should establish a baseline, not a rate")
	}
	// Spendable cash unchanged, but a thousand credits of ore arrived.
	sampleIncome(env(incomeSampleTicks, 0, 2000))
	got, _ := mem["incomeRate"].(int)
	if got != 1000 {
		t.Errorf("incomeRate = %d, want 1000: ore in the silos is income", got)
	}
	if RuleEnv(env(incomeSampleTicks, 0, 2000)).IncomeRate() != 1000 {
		t.Error("IncomeRate() disagrees with what sampleIncome stored")
	}
}
