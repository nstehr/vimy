package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
)

// A subprocess rather than a library, matching how vimy-core compiles a
// doctrine: the boundary is a rule set and a state stream in, a report out, and
// the analysis belongs to the compiler that owns the syntax.
type report struct {
	States     int               `json:"states"`
	Rules      []ruleReport      `json:"rules"`
	Thresholds []thresholdReport `json:"thresholds"`
}

type ruleReport struct {
	Rule      string         `json:"rule"`
	Category  string         `json:"category"`
	Action    string         `json:"action"`
	Seen      int            `json:"seen"`
	Held      int            `json:"held"`
	Preempted int            `json:"preempted"`
	Blocked   int            `json:"blocked"`
	Culprit   *int           `json:"culprit"`
	Clauses   []clauseReport `json:"clauses"`
}

type clauseReport struct {
	Index   int    `json:"index"`
	Blocked int    `json:"blocked"`
	Sole    int    `json:"sole"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Source  string `json:"source"`
}

// Replaying a game window by window.
//
// A game runs under many doctrines — 37 in the game of 7 Sep — and each compiles
// to its own rule set with its own thresholds. Replaying every state against one
// doctrine answers a question nobody asked. So the states are split by the rule
// set that was actually in force and each group is replayed against it.
//
// The pairing is exact because the recording says which set it ran: every case
// carries the fingerprint of its rule set. Inferring it does not work — a
// doctrine takes effect after it is archived, and two doctrines can differ only
// in a threshold inside a comparison, which no amount of matching on names or
// recorded state can see.

type exportFile struct {
	States []json.RawMessage `json:"states"`
	Cases  []exportCase      `json:"cases"`
}

type exportCase struct {
	State   int    `json:"state"`
	Tick    int    `json:"tick"`
	RuleSet string `json:"rule_set"`
}

// window is one doctrine, the rule set it compiled to, and the states recorded
// while it was in force.
type window struct {
	Doctrine rules.Doctrine
	Tick     int
	Rating   string
	States   []json.RawMessage
}

// Replay is a whole game, replayed.
type Replay struct {
	Game     store.ReplayableGame
	Windows  int
	Matched  int // states paired to a doctrine
	Orphaned int // states whose rule set is not among the archived doctrines
	// True when the rule sources have changed since the game was played, so
	// states were paired to doctrines by tick rather than by fingerprint.
	Approximate bool
	Report      report
	// The doctrines the strategist chose, in the order they took effect.
	DoctrineNames []string
	// The doctrines themselves, with the strategist's reasoning. What links the
	// directive to the weights.
	Doctrines []rules.Doctrine
	// What each rule actually did in the game, from the archive rather than the
	// replay. A replay reads sampled states, so a rule that fired a handful of
	// times across a whole game can be absent from every sample and look as
	// though it never could fire.
	Firings map[string]store.Firing
	// Per-window block rates, for telling a rule-set problem from a doctrine
	// that priced itself out.
	Windows_ []windowStats
}

// replayGame pairs a game's states to its doctrines and blames each window
// against the rules that were actually running.
func replayGame(ctx context.Context, st *store.Store, g store.ReplayableGame, rulesDir, bin string) (*Replay, error) {
	// Compressed or not: recordings made before compression must stay readable.
	raw, err := rules.ReadExport(expand(g.ExportPath))
	if err != nil {
		return nil, fmt.Errorf("export for game %d: %w", g.ID, err)
	}
	var exp exportFile
	if err := json.Unmarshal(raw, &exp); err != nil {
		return nil, fmt.Errorf("parse export: %w", err)
	}

	windows, err := st.DoctrinesForGame(ctx, g.ID)
	if err != nil {
		return nil, err
	}
	if len(windows) == 0 {
		return nil, fmt.Errorf("game %d has no archived doctrines", g.ID)
	}

	compiler, err := rules.NewVimycCompiler(bin)
	if err != nil {
		return nil, fmt.Errorf("vimyc: %w", err)
	}
	// The same sources the blame runs against. Without this the fingerprints
	// come from whatever was embedded when this binary was built, and a replay
	// can blame against one rule set while pairing against another.
	compiler.RulesDir = rulesDir

	// Fingerprint every rule set the game could have been running, including the
	// seed set the engine starts on before the first swap.
	byID := map[string]*window{}
	var names []string
	var chosen []rules.Doctrine
	if seed, err := rules.SeedRules(); err == nil {
		byID[rules.RuleSetID(seed)] = &window{Tick: 0, Rating: "seed"}
	}
	for _, w := range windows {
		var d rules.Doctrine
		if err := json.Unmarshal([]byte(w.DoctrineJSON), &d); err != nil {
			continue // a doctrine that will not parse cannot be compiled either
		}
		rs, err := compiler.Compile(d)
		if err != nil {
			return nil, fmt.Errorf("compile %q: %w", d.Name, err)
		}
		byID[rules.RuleSetID(rs)] = &window{Doctrine: d, Tick: w.Tick, Rating: w.Rating}
		names = append(names, d.Name)
		chosen = append(chosen, d)
	}

	// Each state belongs to the rule set that was running when it was recorded.
	// A state appears in many cases; the fingerprint is the same across all of
	// them, so the first one wins.
	seen := make(map[int]bool, len(exp.States))
	rep := &Replay{Game: g}
	for _, c := range exp.Cases {
		if seen[c.State] || c.State >= len(exp.States) {
			continue
		}
		seen[c.State] = true
		w, ok := byID[c.RuleSet]
		if !ok {
			rep.Orphaned++
			continue
		}
		w.States = append(w.States, exp.States[c.State])
		rep.Matched++
	}

	// Nothing matched: the `.vy` sources have been edited since this game, so
	// recompiling its doctrines produces rule sets it never ran. The archive
	// records the doctrine but not the artifact it compiled to, so an exact
	// replay of an older game is not recoverable.
	//
	// Falling back to pairing by tick — the doctrine of the moment, applied to
	// today's rules. Wrong in a knowable way, and better than replaying a whole
	// game against one doctrine, so it is offered and labelled rather than
	// refused.
	// Tested on doctrine windows rather than on matched states: a game starts on
	// the seed rule set, whose fingerprint is stable across source edits, so a
	// seed match alone would look like success while leaving nothing to replay.
	paired := 0
	for _, w := range byID {
		if w.Rating != "seed" {
			paired += len(w.States)
		}
	}
	if paired == 0 {
		rep.Approximate = true
		rep.Orphaned = 0
		rep.Matched = 0
		for _, w := range byID {
			w.States = nil
		}
		byTick := make([]*window, 0, len(byID))
		for _, w := range byID {
			if w.Rating != "seed" {
				byTick = append(byTick, w)
			}
		}
		sort.SliceStable(byTick, func(i, j int) bool { return byTick[i].Tick < byTick[j].Tick })
		if len(byTick) == 0 {
			return nil, fmt.Errorf("game %d: no doctrines could be compiled", g.ID)
		}
		firstTick := map[int]int{}
		for _, c := range exp.Cases {
			if _, ok := firstTick[c.State]; !ok {
				firstTick[c.State] = c.Tick
			}
		}
		for i := range exp.States {
			tick, ok := firstTick[i]
			if !ok {
				continue
			}
			// The window in force is the last one that took effect at or
			// before this tick.
			w := byTick[0]
			for _, cand := range byTick {
				if cand.Tick <= tick {
					w = cand
				}
			}
			w.States = append(w.States, exp.States[i])
			rep.Matched++
		}
	}

	merged := newMerger()
	for _, w := range byID {
		if len(w.States) == 0 || w.Rating == "seed" {
			continue // the seed set is not the doctrine's rule set
		}
		rep.Windows++
		out, err := blameWindow(w, rulesDir, bin)
		if err != nil {
			return nil, err
		}
		merged.addWindow(out, w.Doctrine.Name, rules.DoctrineParams(w.Doctrine), len(w.States), w.States)
	}
	if rep.Windows == 0 {
		return nil, fmt.Errorf("game %d: no states paired to any archived doctrine", g.ID)
	}
	// From the archive, not the replay: the two disagree wherever sampling
	// missed a rule, and the disagreement is worth showing rather than hiding.
	if firings, err := st.FiringsForGame(ctx, g.ID); err == nil {
		rep.Firings = firings
	}
	rep.Report = merged.result(rep.Matched)
	rep.Windows_ = merged.windows
	rep.DoctrineNames = names
	rep.Doctrines = chosen
	return rep, nil
}

// blameWindow replays one doctrine's states against the rules it compiled to.
func blameWindow(w *window, rulesDir, bin string) (report, error) {
	return blameStates(rules.DoctrineParams(w.Doctrine), w.States, rulesDir, bin)
}

// blameStates is blameWindow with the parameters given rather than derived, so
// a sweep can override one input and leave the rest as they were.
func blameStates(params map[string]float64, raw []json.RawMessage, rulesDir, bin string) (report, error) {
	dir, err := os.MkdirTemp("", "currie-")
	if err != nil {
		return report{}, fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	encoded, err := json.Marshal(params)
	if err != nil {
		return report{}, fmt.Errorf("params: %w", err)
	}
	paramsPath := filepath.Join(dir, "params.json")
	if err := os.WriteFile(paramsPath, encoded, 0o600); err != nil {
		return report{}, fmt.Errorf("write params: %w", err)
	}
	states, err := json.Marshal(struct {
		States []json.RawMessage `json:"states"`
	}{raw})
	if err != nil {
		return report{}, fmt.Errorf("states: %w", err)
	}
	statesPath := filepath.Join(dir, "states.json")
	if err := os.WriteFile(statesPath, states, 0o600); err != nil {
		return report{}, fmt.Errorf("write states: %w", err)
	}

	args, err := ruleArgs(rulesDir)
	if err != nil {
		return report{}, err
	}
	args = append(args, "--params", paramsPath, "--blame", statesPath)
	stdout, err := runVimyc(bin, args)
	if err != nil {
		return report{}, err
	}
	var out report
	if err := json.Unmarshal(stdout, &out); err != nil {
		return report{}, fmt.Errorf("parse blame: %w", err)
	}
	return out, nil
}

// ruleArgs is every .vy source bar the seed set, which is its own rule set.
func ruleArgs(dir string) ([]string, error) {
	sources, err := filepath.Glob(filepath.Join(dir, "*.vy"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		if filepath.Base(s) != "seed.vy" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no .vy sources", dir)
	}
	return out, nil
}

// merger sums per-window reports into one.
//
// Keyed on rule name and clause index rather than position: specialisation drops
// rules a doctrine cannot use, so two windows rarely emit the same list. The
// clause index is stable for a rule that survives, because every window compiles
// the same `.vy` sources and only the parameters differ.
type merger struct {
	rules      map[string]*ruleReport
	order      []string
	thresholds []thresholdReport
	// Per-window block rates, kept alongside the sums. Summing answers "what
	// stopped this rule"; keeping the windows apart answers the more useful
	// question of whether it stopped it because of how the rules are written or
	// because of what the strategist asked for that window.
	windows []windowStats
}

// windowStats is one doctrine window: what it asked for, and how often each
// clause was false while it was in force.
type windowStats struct {
	Doctrine string
	Params   map[string]float64
	States   int
	// The states themselves, kept so a sweep can replay them under a doctrine
	// input the strategist never tried.
	Raw []json.RawMessage
	// Keyed "rule\x00clauseIndex".
	Blocked map[string]int
}

func newMerger() *merger { return &merger{rules: map[string]*ruleReport{}} }

func clauseKey(rule string, index int) string {
	return fmt.Sprintf("%s\x00%d", rule, index)
}

// addWindow records one window's blame, both into the totals and as its own
// row for the sensitivity analysis.
func (m *merger) addWindow(r report, name string, params map[string]float64, states int, raw []json.RawMessage) {
	ws := windowStats{Doctrine: name, Params: params, States: states, Blocked: map[string]int{}, Raw: raw}
	for _, in := range r.Rules {
		for _, c := range in.Clauses {
			if c.Blocked > 0 {
				ws.Blocked[clauseKey(in.Rule, c.Index)] = c.Blocked
			}
		}
	}
	m.windows = append(m.windows, ws)
	m.add(r)
}

func (m *merger) add(r report) {
	// Gates are kept per window, not merged. A curve's candidate values come
	// from the distribution of that window's states, so two windows rarely
	// offer the same ones and adding them position by position sums counts
	// taken at different thresholds. Relief is computed per window and summed
	// afterwards, where it is well defined.
	m.thresholds = append(m.thresholds, r.Thresholds...)
	for _, in := range r.Rules {
		cur, ok := m.rules[in.Rule]
		if !ok {
			cp := in
			cp.Clauses = append([]clauseReport(nil), in.Clauses...)
			m.rules[in.Rule] = &cp
			m.order = append(m.order, in.Rule)
			continue
		}
		cur.Seen += in.Seen
		cur.Held += in.Held
		cur.Preempted += in.Preempted
		cur.Blocked += in.Blocked
		for i := range in.Clauses {
			if i >= len(cur.Clauses) {
				cur.Clauses = append(cur.Clauses, in.Clauses[i])
				continue
			}
			cur.Clauses[i].Blocked += in.Clauses[i].Blocked
			cur.Clauses[i].Sole += in.Clauses[i].Sole
		}
	}
}

func (m *merger) result(states int) report {
	out := report{States: states}
	for _, name := range m.order {
		r := m.rules[name]
		// Recomputed after merging: the culprit of the whole game is not
		// necessarily the culprit of any one window.
		r.Culprit = nil
		best := -1
		for i, c := range r.Clauses {
			if c.Sole > 0 && (best < 0 || c.Sole > r.Clauses[best].Sole) {
				best = i
			}
		}
		if best >= 0 {
			idx := best
			r.Culprit = &idx
		}
		out.Rules = append(out.Rules, *r)
	}
	out.Thresholds = m.thresholds
	sort.SliceStable(out.Rules, func(i, j int) bool {
		return out.Rules[i].Blocked > out.Rules[j].Blocked
	})
	return out
}

// runVimyc compiles the rule set and replays the states, returning the report.
//
// Warnings on stderr — shared priorities, shadowed rules — are findings about
// the rule set rather than failures, so they are passed through rather than
// swallowed or treated as an error.
func runVimyc(bin string, args []string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("vimyc: %w: %s", err, strings.TrimSpace(errs.String()))
	}
	for _, line := range strings.Split(strings.TrimSpace(errs.String()), "\n") {
		if line != "" {
			fmt.Fprintln(os.Stderr, line)
		}
	}
	return out.Bytes(), nil
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// print ranks by how often one clause was solely responsible: that is the
// number a person can act on, and ranking by raw blocked count would put
// rules blocked by several things at once — which no single edit frees — at
// the top.
