package rules

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A nil exporter must be free and silent: it is the default, and Evaluate runs
// on every tick of a live game.
func TestNilExporterIsInert(t *testing.T) {
	var e *StateExporter
	if idx := e.begin(RuleEnv{}, nil); idx != -1 {
		t.Fatalf("nil exporter began a recording: %d", idx)
	}
	e.record(-1, 0, "r", true, false)
	if err := e.Flush(); err != nil {
		t.Fatalf("nil exporter flush: %v", err)
	}
}

func TestExporterSamplesAndRecords(t *testing.T) {
	dir := t.TempDir()
	// Every third evaluation, so the sampling is observable.
	exp, err := NewStateExporter(dir, 3, 1000)
	if err != nil {
		t.Fatal(err)
	}

	rng := rand.New(rand.NewSource(11))
	rules, err := compileRules(DefaultRules())
	if err != nil {
		t.Fatal(err)
	}

	const evaluations = 9
	for i := 0; i < evaluations; i++ {
		gs := generateState(rng)
		env := RuleEnv{State: gs, Faction: "soviet", Memory: map[string]any{}}
		updateIntel(env)
		updateBuiltRoles(env)
		updateSquads(env)

		idx := exp.begin(env, rules)
		firedCategories := map[string]bool{}
		for _, r := range rules {
			if firedCategories[r.Category] {
				exp.record(idx, gs.Tick, r.Name, false, true)
				continue
			}
			exp.record(idx, gs.Tick, r.Name, false, false)
		}
	}

	if err := exp.Flush(); err != nil {
		t.Fatal(err)
	}

	written, err := filepath.Glob(filepath.Join(dir, "export-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("wrote %d files, want 1", len(written))
	}
	b, err := os.ReadFile(written[0])
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		States []map[string]any `json:"states"`
		Cases  []ExportedCase   `json:"cases"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}

	// 9 evaluations at one in three.
	if len(got.States) != 3 {
		t.Errorf("sampled %d states, want 3", len(got.States))
	}
	if want := 3 * len(rules); len(got.Cases) != want {
		t.Errorf("recorded %d cases, want %d", len(got.Cases), want)
	}
	for _, c := range got.Cases {
		if c.State < 0 || c.State >= len(got.States) {
			t.Fatalf("case %q references state %d of %d", c.Rule, c.State, len(got.States))
		}
	}
}

func TestExporterRespectsItsCap(t *testing.T) {
	exp, err := NewStateExporter(t.TempDir(), 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(12))

	for i := 0; i < 20; i++ {
		env := RuleEnv{State: generateState(rng), Faction: "soviet", Memory: map[string]any{}}
		if idx := exp.begin(env, nil); idx >= 0 {
			exp.record(idx, i, "r", true, false)
		}
	}
	if len(exp.cases) > 5 {
		t.Errorf("recorded %d cases past a cap of 5", len(exp.cases))
	}
}

// Nothing recorded means nothing written, so a game played without the flag
// leaves no file behind.
func TestExporterWritesNothingWhenIdle(t *testing.T) {
	dir := t.TempDir()
	exp, err := NewStateExporter(dir, 1000, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := exp.Flush(); err != nil {
		t.Fatal(err)
	}
	written, _ := filepath.Glob(filepath.Join(dir, "export-*.json"))
	if len(written) != 0 {
		t.Errorf("wrote %d files with nothing recorded", len(written))
	}
}

// An empty dir means ~/.vimy/exports, the same place the database lives.
func TestDefaultsAlongsideTheDatabase(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	exp, err := NewStateExporter("", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".vimy", "exports"); exp.Dir() != want {
		t.Errorf("Dir() = %q, want %q", exp.Dir(), want)
	}
}

func TestExpandsHome(t *testing.T) {
	// A shell does not expand `~` inside quotes, so it arrives literally.
	exp, err := NewStateExporter("~/.vimy/exports", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if want := filepath.Join(home, ".vimy", "exports"); exp.Dir() != want {
		t.Errorf("Dir() = %q, want %q", exp.Dir(), want)
	}
}

func TestWritableLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	exp, err := NewStateExporter(dir, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := exp.Writable(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("the writability probe left %d entries behind", len(entries))
	}
}

// A second game in the same process writes its own file rather than adding to
// the first one's.
func TestEachGameWritesItsOwnFile(t *testing.T) {
	dir := t.TempDir()
	exp, err := NewStateExporter(dir, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(13))
	for game := 0; game < 2; game++ {
		env := RuleEnv{State: generateState(rng), Faction: "soviet", Memory: map[string]any{}}
		if idx := exp.begin(env, nil); idx >= 0 {
			exp.record(idx, game, "r", true, false)
		}
		if err := exp.Flush(); err != nil {
			t.Fatal(err)
		}
		// Timestamps are second-resolution, so a same-second second flush would
		// overwrite the first.
		time.Sleep(1100 * time.Millisecond)
	}
	written, _ := filepath.Glob(filepath.Join(dir, "export-*.json"))
	if len(written) != 2 {
		t.Errorf("two games wrote %d files, want 2", len(written))
	}
}

// A projection asks only what the loaded rules ask. The union over every
// doctrine asks about dozens of `overextended-squad-members` thresholds that no
// single rule set uses, which is 15x the work and most of the file size.
func TestProjectsOnlyWhatTheRulesAsk(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	env := RuleEnv{State: generateState(rng), Faction: "soviet", Memory: map[string]any{}}
	updateIntel(env)
	updateBuiltRoles(env)
	updateSquads(env)

	seed, err := compileRules(DefaultRules())
	if err != nil {
		t.Fatal(err)
	}

	narrow := projectFor(env, seed)
	wide := project(env)

	if len(narrow.Collections) >= len(wide.Collections) {
		t.Errorf("projecting for the seed rules asked %d collection keys, the union asked %d",
			len(narrow.Collections), len(wide.Collections))
	}
	// Everything the seed rules do ask about must still be answered.
	for k := range narrow.Collections {
		if _, ok := wide.Collections[k]; !ok {
			t.Errorf("narrow projection asked %q, which the union did not", k)
		}
	}
}
