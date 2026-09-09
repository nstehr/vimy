package rules

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Recording real evaluations for vimyc.
//
// The offline corpus is hand-generated, and hand-generated states don't grow
// accumulated intel, formed squads or threat fields — so the predicates reading
// them go barely exercised. Real games do.
//
// Off unless a path is set, and sampled when on: projecting a state costs ~1ms
// against ~17us to evaluate the seed rules.

// ExportedCase is one rule's evaluation against the state as it stood. Per rule
// rather than per tick: actions mutate Memory mid-loop and later rules read it.
type ExportedCase struct {
	Tick int    `json:"tick"`
	Rule string `json:"rule"`
	// The rule set in force, recorded rather than inferred later: archival and
	// activation times differ, and neither rule names nor the questions a state
	// asks can separate two doctrines differing only in a threshold.
	RuleSet string `json:"rule_set"`
	State   int    `json:"state"`
	Fired   bool   `json:"fired"`
	// Blocked by an exclusive rule in its category, so never evaluated. Recorded
	// anyway — whether it would have fired is the counterfactual worth having,
	// and it can't be recovered later.
	Skipped bool `json:"skipped"`
}

// StateExporter accumulates evaluations and writes them at game end. States are
// stored once and referenced by index; inlining them per rule cost 21MB against
// under two.
type StateExporter struct {
	mu       sync.Mutex
	dir      string
	every    int
	seen     int
	states   []vimycState
	cases    []ExportedCase
	maxCases int
}

// NewStateExporter records one evaluation in every `every`, up to `maxCases`.
// An empty dir means ~/.vimy/exports.
//
// Sampled because projection is expensive, not because the data is redundant —
// a few hundred real states beat any number of generated ones.
func NewStateExporter(dir string, every, maxCases int) (*StateExporter, error) {
	if every < 1 {
		every = 1
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		dir = filepath.Join(home, ".vimy", "exports")
	} else {
		dir = expandHome(dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return &StateExporter{dir: dir, every: every, maxCases: maxCases}, nil
}

// expandHome resolves a leading `~`, which the shell won't inside quotes.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// begin decides whether this evaluation is sampled; negative means skip.
//
// Projects against the loaded rules, not everything a doctrine could emit: the
// union asks 38 overextended-squad-members thresholds where a live rule set
// uses two.
func (e *StateExporter) begin(env RuleEnv, rules []*Rule) int {
	if e == nil {
		return -1
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.seen++
	if e.seen%e.every != 0 || len(e.cases) >= e.maxCases {
		return -1
	}
	return e.snapshot(env, rules)
}

// snapshot records the env as it stands and returns the state's index.
//
// Called again after each firing, because an action mutates Memory and every
// later rule in the tick sees the change — one state per tick made vimyc
// disagree with expr on exactly the rules that ran after a firing. Only firings
// change anything, so this is two or three projections a tick, not eighty.
func (e *StateExporter) snapshot(env RuleEnv, rules []*Rule) int {
	e.states = append(e.states, projectFor(env, rules))
	return len(e.states) - 1
}

// refresh re-projects after a firing when the evaluation is being recorded.
// Callers pass their previous index and use whatever comes back.
func (e *StateExporter) refresh(prev int, env RuleEnv, rules []*Rule) int {
	if e == nil || prev < 0 {
		return prev
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshot(env, rules)
}

func (e *StateExporter) record(stateIdx, tick int, ruleSet, rule string, fired, skipped bool) {
	if e == nil || stateIdx < 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cases = append(e.cases, ExportedCase{
		Tick: tick, Rule: rule, RuleSet: ruleSet, State: stateIdx,
		Fired: fired, Skipped: skipped,
	})
}

// Dir is the resolved output directory.
func (e *StateExporter) Dir() string {
	if e == nil {
		return ""
	}
	return e.dir
}

// Writable fails a bad output location at startup, not after a played game.
func (e *StateExporter) Writable() error {
	if e == nil {
		return nil
	}
	probe := filepath.Join(e.dir, ".write-probe")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Remove(probe)
}

// RuleSetID fingerprints a rule set by everything that decides its behavior:
// conditions, priority and exclusivity. Names alone won't do — two doctrines
// routinely emit the same rules with different thresholds.
//
// Order-independent of necessity: the engine holds rules in priority-sorted
// order, and the sort isn't stable, so two sorts of the same rules need not
// agree with each other.
func RuleSetID(rules []*Rule) string {
	lines := make([]string, 0, len(rules))
	for _, r := range rules {
		lines = append(lines, fmt.Sprintf("%s\x00%d\x00%s\x00%t\x00%s\x00%s",
			r.Name, r.Priority, r.Category, r.Exclusive, r.ConditionSrc, r.ActionSrc))
	}
	sort.Strings(lines)

	h := sha256.New()
	for _, l := range lines {
		fmt.Fprintf(h, "%s\x1e", l)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Flush writes what was recorded and returns the file, empty if nothing was.
// The archive stores that path: replay pairs an export to its doctrines, and
// that pairing should be recorded rather than reconstructed from timestamps.
func (e *StateExporter) Flush() (string, error) {
	if e == nil {
		return "", nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.cases) == 0 {
		return "", nil
	}

	payload := struct {
		States []vimycState   `json:"states"`
		Cases  []ExportedCase `json:"cases"`
	}{e.states, e.cases}

	b, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal export: %w", err)
	}
	// Named at write time, not startup, so a sidecar left running across several
	// games produces several files.
	//
	// Compressed because nearly all of it is the same predicate names again, once
	// per state — forty times smaller, better than interning would manage and
	// with no schema to agree on.
	path := filepath.Join(e.dir, fmt.Sprintf("export-%s.json.gz", time.Now().UTC().Format("20060102-150405")))
	if err := writeGzip(path, append(b, '\n')); err != nil {
		return "", err
	}
	slog.Info("exported rule evaluations",
		"path", path, "states", len(e.states), "cases", len(e.cases))

	// Cleared so a second game in the same process doesn't re-write the first.
	e.states, e.cases, e.seen = nil, nil, 0
	return path, nil
}

// writeGzip goes via a temp file: a crash mid-write would otherwise leave a
// truncated export that reads as a short game.
func writeGzip(path string, b []byte) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	zw, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("gzip: %w", err)
	}
	if _, err := zw.Write(b); err != nil {
		zw.Close()
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := zw.Close(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("flush %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	return os.Rename(tmp, path)
}

// ReadExport reads an export, compressed or not — the plain files predate
// compression and are worth a branch to keep readable.
func ReadExport(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
		return b, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}
