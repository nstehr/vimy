package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
// The offline corpus is built from hand-generated game states, which cannot
// realistically produce accumulated intel, formed squads or threat fields — so
// the predicates that read them are barely exercised. Real games can.
//
// Off unless a path is set. Projecting costs about 1ms against 17us to evaluate
// the seed rules, so this samples rather than recording every evaluation.

// ExportedCase is one rule's evaluation, with the state as it stood at that
// moment. Per rule rather than per tick because actions mutate Memory as the
// loop runs and later rules read it — see vimyc/docs/design.md.
type ExportedCase struct {
	Tick int    `json:"tick"`
	Rule string `json:"rule"`
	// Identifies the rule set in force, so a recording can be paired with the
	// conditions that produced it.
	//
	// Inferring it afterwards does not quite work: a doctrine is archived when
	// generated and takes effect once the LLM call lands, and matching on rule
	// names or on the questions a state asks cannot separate two doctrines whose
	// only difference is a threshold inside a comparison. That left one
	// disagreement in 12,442 that was pairing noise rather than a real one.
	RuleSet string `json:"rule_set"`
	State   int    `json:"state"`
	Fired   bool   `json:"fired"`
	// Blocked by an exclusive rule in the same category, so it was never
	// evaluated. Recorded anyway: "would this have fired had the category been
	// free?" is exactly the counterfactual worth asking, and it cannot be
	// recovered afterwards.
	Skipped bool `json:"skipped"`
}

// StateExporter accumulates evaluations and writes them at game end.
//
// States are stored once and referenced by index. Inlining each into every one
// of its rules made the offline corpus 21MB instead of under two.
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
//
// An empty dir means ~/.vimy/exports, alongside the database. Each game writes
// its own timestamped file: a fixed path would mean every game silently
// overwrote the last, and the whole point is to accumulate real ones.
//
// Sampling because the projection is the expensive part, not because the data
// is redundant: a few hundred real states beat any number of generated ones.
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

// expandHome resolves a leading `~`, which a shell does not when the path
// arrives inside quotes.
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

// begin decides whether this evaluation is sampled. A negative result means
// skip; otherwise `snapshot` gives the state to record against.
//
// Projects against the rules actually loaded, not against everything a doctrine
// could emit. The union asks about 38 different `overextended-squad-members`
// thresholds where a live rule set uses two, which was 15x the work and 11KB a
// state instead of two.
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
// Called again after a rule fires. An action mutates Memory — FormSquad assigns
// units, so UnassignedIdleGround drops — and every later rule in the tick sees
// the change. Recording one state per evaluation made vimyc disagree with expr
// on 36 of 19,925 real evaluations, every one a rule that ran after a firing.
//
// Only a firing can change anything, so this costs one projection per firing
// rather than one per rule: two or three a tick, not eighty.
func (e *StateExporter) snapshot(env RuleEnv, rules []*Rule) int {
	e.states = append(e.states, projectFor(env, rules))
	return len(e.states) - 1
}

// refresh re-projects after a rule has fired, if this evaluation is being
// recorded. Callers pass the previous index and use whatever comes back.
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

// Writable reports whether the directory can be written, so a bad location
// fails at startup rather than after a game has already been played.
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

// RuleSetID fingerprints a rule set by everything that decides how it behaves.
//
// Names alone are not enough: two doctrines routinely emit the same rules with
// different thresholds. Conditions are, and priority and exclusivity go in too
// since they decide what runs and what gets blocked.
//
// Order-independent, because it has to be. `compileRules` sorts by priority
// with `sort.Slice`, so the engine holds a different ordering from what
// the compiler returned — and the sort is not stable, so two sorts of the
// same rules need not even agree with each other.
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

// Flush writes what was recorded. Safe to call with nothing recorded.
func (e *StateExporter) Flush() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.cases) == 0 {
		return nil
	}

	payload := struct {
		States []vimycState   `json:"states"`
		Cases  []ExportedCase `json:"cases"`
	}{e.states, e.cases}

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal export: %w", err)
	}
	// One file per game, named when it is written rather than at startup, so a
	// sidecar left running across several games produces several files.
	path := filepath.Join(e.dir, fmt.Sprintf("export-%s.json", time.Now().UTC().Format("20060102-150405")))
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	slog.Info("exported rule evaluations",
		"path", path, "states", len(e.states), "cases", len(e.cases))

	// Cleared so a second game in the same process starts fresh rather than
	// re-writing everything the first one saw.
	e.states, e.cases, e.seen = nil, nil, 0
	return nil
}
