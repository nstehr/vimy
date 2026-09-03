package rules

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	_ "modernc.org/sqlite"
)

// Pairs a recorded game with the rule sets that produced it, so vimyc can be
// checked against real play rather than against generated states.
//
// The export records rule names and outcomes but not conditions — a rule's
// condition depends on the doctrine active at that tick, and a game swaps
// doctrine dozens of times. archived_doctrines holds those, keyed by the tick
// they took effect, so the pairing is recoverable after the fact.
//
//	EXPORT=~/.vimy/exports/export-….json GAME=66 \
//	  DUMP_DIR=../../../vimyc/testdata go test -run TestBuildRealDifferential ./rules/
//
// Not part of the normal suite: it needs a recorded game and the database.

type realCorpus struct {
	// One vimyc source file per distinct doctrine window the export covers.
	RuleSets []string `json:"rule_sets"`
	// Verbatim from the export.
	States []json.RawMessage `json:"states"`
	Cases  []realCase        `json:"cases"`
}

type realCase struct {
	Tick    int    `json:"tick"`
	Rule    string `json:"rule"`
	State   int    `json:"state"`
	RuleSet int    `json:"rule_set"`
	Fired   bool   `json:"fired"`
	Skipped bool   `json:"skipped"`
}

type exportFile struct {
	States []json.RawMessage `json:"states"`
	Cases  []struct {
		Tick    int    `json:"tick"`
		Rule    string `json:"rule"`
		RuleSet string `json:"rule_set"`
		State   int    `json:"state"`
		Fired   bool   `json:"fired"`
		Skipped bool   `json:"skipped"`
	} `json:"cases"`
}

// doctrineWindow is a doctrine and the tick it took effect.
type doctrineWindow struct {
	tick     int
	doctrine Doctrine
}

func loadWindows(t *testing.T, dbPath string, gameID int) []doctrineWindow {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open %s: %v", dbPath, err)
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT tick, doctrine_json FROM archived_doctrines WHERE game_id = ? ORDER BY tick`,
		gameID)
	if err != nil {
		t.Fatalf("query doctrines: %v", err)
	}
	defer rows.Close()

	var out []doctrineWindow
	for rows.Next() {
		var tick int
		var raw string
		if err := rows.Scan(&tick, &raw); err != nil {
			t.Fatal(err)
		}
		var d Doctrine
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			t.Fatalf("tick %d: %v", tick, err)
		}
		d.Validate()
		out = append(out, doctrineWindow{tick: tick, doctrine: d})
	}
	return out
}

func TestBuildRealDifferential(t *testing.T) {
	out := os.Getenv("DUMP_DIR")
	exportPath := os.Getenv("EXPORT")
	if out == "" || exportPath == "" {
		t.Skip("needs DUMP_DIR and EXPORT")
	}
	gameID := 0
	if _, err := fmt.Sscanf(os.Getenv("GAME"), "%d", &gameID); err != nil || gameID == 0 {
		t.Fatal("needs GAME=<game_id>")
	}

	raw, err := os.ReadFile(os.ExpandEnv(exportPath))
	if err != nil {
		t.Fatal(err)
	}
	var exp exportFile
	if err := json.Unmarshal(raw, &exp); err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	windows := loadWindows(t, filepath.Join(home, ".vimy", "vimy.db"), gameID)
	if len(windows) == 0 {
		t.Fatalf("no archived doctrines for game %d", gameID)
	}

	untranslatable := map[string]int{}
	unmatched := map[string]int{}

	// Paired on the fingerprint the export carries. Earlier versions inferred
	// the rule set from ticks, then from rule names, then from the questions a
	// state asked; each was closer and none was exact, because a doctrine takes
	// effect after it is archived and two doctrines can differ only in a
	// threshold inside a comparison. The recording now says which set it ran.
	byID := map[string]string{} // fingerprint -> vimyc source
	addCandidate := func(rs []*Rule) {
		src, err := ToVimyc(rs)
		if err != nil {
			untranslatable[err.Error()]++
			return
		}
		byID[RuleSetID(rs)] = src
	}
	addCandidate(DefaultRules()) // the engine starts here, before the first swap
	for _, w := range windows {
		addCandidate(CompileDoctrine(w.doctrine))
	}

	corpus := realCorpus{States: exp.States}
	kept := map[string]int{}
	for _, c := range exp.Cases {
		src, ok := byID[c.RuleSet]
		if !ok {
			unmatched[c.RuleSet]++
			continue
		}
		idx, seen := kept[c.RuleSet]
		if !seen {
			corpus.RuleSets = append(corpus.RuleSets, src)
			idx = len(corpus.RuleSets) - 1
			kept[c.RuleSet] = idx
		}
		corpus.Cases = append(corpus.Cases, realCase{
			Tick: c.Tick, Rule: c.Rule, State: c.State,
			RuleSet: idx, Fired: c.Fired, Skipped: c.Skipped,
		})
	}

	for e, n := range untranslatable {
		t.Errorf("%d rule sets failed to translate: %s", n, e)
	}
	if len(unmatched) > 0 {
		var ids []string
		for id := range unmatched {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		// A fingerprint the archived doctrines cannot reproduce means the
		// recording and the database disagree about what ran, which is worth
		// knowing rather than skipping.
		t.Errorf("%d rule set fingerprints matched no archived doctrine, e.g. %s (%d cases)",
			len(ids), ids[0], unmatched[ids[0]])
	}

	b, err := json.Marshal(corpus)
	if err != nil {
		t.Fatal(err)
	}
	path := out + "/real_differential.json"
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d rule sets, %d states, %d cases to %s",
		len(corpus.RuleSets), len(corpus.States), len(corpus.Cases), path)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
