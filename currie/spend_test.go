package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nstehr/vimy/vimy-core/store"
)

// The rule table and the engine are the two halves that can drift apart, so
// the test that matters is that every rule the table names is one the engine
// still prices.
func TestRuleItemsArePricedByTheEngine(t *testing.T) {
	items, err := loadRuleItems()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("no rule items")
	}
	dir := filepath.Join("..", "engine", "mods", "ra", "rules")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no engine checkout")
	}
	prices, err := enginePrices(dir)
	if err != nil {
		t.Fatal(err)
	}
	for rule, it := range items {
		if _, ok := prices[it.Item]; !ok {
			t.Errorf("%s buys %q, which the engine does not price", rule, it.Item)
		}
	}
}

func TestEnginePricesReadsTheFirstCostPerActor(t *testing.T) {
	dir := t.TempDir()
	// Shaped like the mod's: an actor, its Cost nested under a trait, and a
	// .Husk variant that must not be mistaken for a second actor.
	yaml := "E1:\n\tInherits: ^Soldier\n\tValued:\n\t\tCost: 100\n" +
		"2TNK:\n\tValued:\n\t\tCost: 850\n" +
		"2TNK.Husk:\n\tValued:\n\t\tCost: 9999\n"
	if err := os.WriteFile(filepath.Join(dir, "units.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	prices, err := enginePrices(dir)
	if err != nil {
		t.Fatal(err)
	}
	for item, want := range map[string]int{"e1": 100, "2tnk": 850} {
		if prices[item] != want {
			t.Errorf("%s = %d, want %d", item, prices[item], want)
		}
	}
}

func TestComputeSpendSkipsRulesThatNeverActed(t *testing.T) {
	items := map[string]ruleItem{
		"produce-vehicle":  {Item: "2tnk", Kind: "armour"},
		"produce-infantry": {Item: "e1", Kind: "infantry"},
		"build-refinery":   {Item: "proc", Kind: "economy"},
	}
	prices := map[string]int{"2tnk": 850, "e1": 100, "proc": 1400}
	firings := map[string]store.Firing{
		"produce-vehicle": {Matched: 500, Acted: 10},
		// Matched plenty and bought nothing.
		"produce-infantry": {Matched: 300, Acted: 0},
		// Predates the counter: -1 is not zero and must not be priced.
		"build-refinery": {Matched: 40, Acted: -1},
		// Not a producing rule at all.
		"retreat-damaged-units": {Matched: 90, Acted: 12},
	}
	s := computeSpend(firings, prices, items, 20000)
	if s.Total != 8500 {
		t.Errorf("total = %d, want 8500", s.Total)
	}
	if len(s.Lines) != 1 || s.Lines[0].Rule != "produce-vehicle" {
		t.Fatalf("lines = %+v, want only produce-vehicle", s.Lines)
	}
	if len(s.Kinds) != 1 || s.Kinds[0].Percent != 100 {
		t.Errorf("kinds = %+v, want armour at 100%%", s.Kinds)
	}
	if s.Earned != 20000 {
		t.Errorf("earned = %d", s.Earned)
	}
}

func TestComputeSpendReportsUnpricedItemsRatherThanDroppingThem(t *testing.T) {
	items := map[string]ruleItem{"produce-spy": {Item: "spy", Kind: "infantry"}}
	s := computeSpend(map[string]store.Firing{"produce-spy": {Acted: 3}}, map[string]int{}, items, 0)
	if s.Total != 0 {
		t.Errorf("total = %d, want 0", s.Total)
	}
	if len(s.Unpriced) != 1 || s.Unpriced[0] != "spy" {
		t.Errorf("unpriced = %v, want [spy]", s.Unpriced)
	}
}

// A rule dead under one doctrine can be live under the next, so the count of
// windows is the finding, not the mere presence of the warning.
func TestWarningSetCountsWindows(t *testing.T) {
	w := newWarningSet()
	w.add([]string{"a can never fire"})
	w.add(nil)
	w.add([]string{"a can never fire", "b and c share priority"})

	got := w.list()
	if len(got) != 2 {
		t.Fatalf("got %d warnings, want 2", len(got))
	}
	if got[0].Text != "a can never fire" || got[0].Windows != 2 || got[0].Total != 3 {
		t.Errorf("first = %+v, want a in 2 of 3", got[0])
	}
	if got[1].Windows != 1 {
		t.Errorf("second = %+v, want 1 window", got[1])
	}
}

func TestTrimSourcePathLeavesJustTheFile(t *testing.T) {
	args := []string{"/tmp/x/vy/micro.vy", "/tmp/x/vy/core.vy"}
	line := "/tmp/x/vy/micro.vy:115:6: warning: `guard-harvesters` can never fire"
	if got := trimSourcePath(line, args); got != "micro.vy:115:6: warning: `guard-harvesters` can never fire" {
		t.Errorf("got %q", got)
	}
}

// The CLI and the page must agree about which rules never ran. They did not:
// the raw blame lists rules the engine recorded acting, because the sample is
// every 15th evaluation and a rule can act between samples. Calling those dead
// produced a false "zero artillery" reading of game 127, where the rule had in
// fact produced five times.
func TestPreemptedExcludesRulesTheEngineSawAct(t *testing.T) {
	rules := []ruleReport{
		{Rule: "produce-vehicle", Category: "produce-vehicle", Seen: 100, Held: 90},
		{Rule: "produce-siege-vehicle", Category: "produce-vehicle", Seen: 100, Preempted: 80},
		// Never held in the sample, but the engine recorded it producing.
		{Rule: "produce-minelayer", Category: "produce-vehicle", Seen: 100, Preempted: 70},
		// Never held and nothing preempted it: a blocked rule, not this list.
		{Rule: "produce-flak-truck", Category: "produce-vehicle", Seen: 100, Blocked: 100},
	}
	firings := map[string]store.Firing{
		"produce-minelayer": {Matched: 47, Acted: 6},
		// -1 is "unmeasured", not "did nothing", and must not exclude a rule.
		"produce-siege-vehicle": {Matched: 40, Acted: -1},
	}
	got := preempted(rules, firings)
	if len(got) != 1 || got[0].Name != "produce-siege-vehicle" {
		t.Fatalf("got %+v, want only produce-siege-vehicle", got)
	}
	if got[0].Preempted != 80 || got[0].Rate != 80 {
		t.Errorf("got %d at %.0f%%, want 80 at 80%%", got[0].Preempted, got[0].Rate)
	}
	if len(got[0].LostTo) != 1 || got[0].LostTo[0] != "produce-vehicle" {
		t.Errorf("lost to %v, want [produce-vehicle]", got[0].LostTo)
	}
}

func TestPreemptedNamesTheMostFrequentWinnerFirst(t *testing.T) {
	rules := []ruleReport{
		{Rule: "rare-winner", Category: "c", Seen: 10, Held: 2},
		{Rule: "usual-winner", Category: "c", Seen: 10, Held: 8},
		{Rule: "loser", Category: "c", Seen: 10, Preempted: 10},
	}
	got := preempted(rules, nil)
	if len(got) != 1 {
		t.Fatalf("got %d rows", len(got))
	}
	if got[0].LostTo[0] != "usual-winner" {
		t.Errorf("lost to %v, want usual-winner first", got[0].LostTo)
	}
}

// A spend estimate larger than the game's earnings is proof the estimate is
// wrong, not a finding about the game. Act counts include produce resends, so
// one unit is counted many times; games 127, 128 and 129 came out at 2.20x,
// 1.46x and 1.97x of everything earned and were quoted as fact for a whole
// session before anyone divided one by the other.
func TestSpendFlagsItselfWhenItExceedsEarnings(t *testing.T) {
	items := map[string]ruleItem{"produce-vehicle": {Item: "2tnk", Kind: "armour"}}
	prices := map[string]int{"2tnk": 850}
	firings := map[string]store.Firing{"produce-vehicle": {Acted: 123}}

	over := computeSpend(firings, prices, items, 96262) // game 129
	if !over.Overstated {
		t.Fatal("104550 spent against 96262 earned must be flagged")
	}
	if over.Inflation < 1.08 || over.Inflation > 1.09 {
		t.Errorf("inflation = %.3f, want ~1.086", over.Inflation)
	}

	// Plausible totals must stay unflagged, or the warning means nothing.
	ok := computeSpend(firings, prices, items, 500000)
	if ok.Overstated || ok.Inflation != 0 {
		t.Errorf("104550 against 500000 earned must not be flagged: %+v", ok)
	}
	// No earnings recorded is not evidence of anything either way.
	none := computeSpend(firings, prices, items, 0)
	if none.Overstated {
		t.Error("an unknown earned figure must not flag the estimate")
	}
}

// The three rally numbers exist to separate two faults, so the means must be
// per-rally and must not divide by zero for a game that never rallied.
func TestRallyMeansSeparateTheTwoFaults(t *testing.T) {
	// 10 rallies: squads of 6, only 2 commandable, furthest 20 cells out.
	// That shape indicts the predicate, not the 8-cell radius.
	o := store.Outcome{RallyCount: 10, RallyMembersSum: 60, RallyIdleSum: 20, RallySpreadSum: 200}
	if got := o.MeanRallyMembers(); got != 6 {
		t.Errorf("members = %.1f, want 6", got)
	}
	if got := o.MeanRallyCommandable(); got != 2 {
		t.Errorf("commandable = %.1f, want 2", got)
	}
	if got := o.MeanRallySpread(); got != 20 {
		t.Errorf("spread = %.1f, want 20", got)
	}

	var never store.Outcome
	if never.MeanRallyMembers() != 0 || never.MeanRallySpread() != 0 {
		t.Error("a game with no rallies must report zero, not divide by zero")
	}
}
