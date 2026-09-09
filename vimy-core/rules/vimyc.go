package rules

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Compiling a doctrine into rules.
//
// A subprocess rather than a library: the boundary is a doctrine in and a rule
// set out, it runs once per doctrine window rather than per tick, and a process
// cannot take the bot down with it.

// The rule set source, carried in the binary so a game needs only the compiler
// on PATH. Hand-written in `rules/vy/`: it is Vimy's strategy the way the `.go`
// files are Vimy's code, and vimyc is the language it is written in.
//
// Split by topic but compiled as one unit. `seed.vy` is embedded with the rest
// and dropped below — it is a separate, standalone rule set.
//
//go:embed vy/*.vy
var doctrineSource embed.FS

// VimycCompiler turns a Doctrine into rules by running the vimyc binary.
type VimycCompiler struct {
	// Path to the binary; "vimyc" finds it on PATH.
	Bin string
	// Where the `.vy` sources come from. Empty uses the embedded copy, which is
	// what a game wants — it should run the rules it shipped with.
	//
	// A replay wants the opposite: Currie blames against sources on disk, and
	// fingerprinting an older embedded copy leaves the two halves of its
	// analysis reading different rule sets.
	RulesDir string
	// Compiling the full rule set takes single-digit milliseconds, so this only
	// ever catches something being wrong.
	Timeout time.Duration
}

// NewVimycCompiler returns a compiler reading the embedded rule set, probing it
// at startup — a missing binary found twenty minutes into a game is a wasted
// game.
func NewVimycCompiler(bin string) (*VimycCompiler, error) {
	if bin == "" {
		bin = "vimyc"
	}
	c := &VimycCompiler{Bin: bin, Timeout: 10 * time.Second}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("vimyc not found: %w", err)
	}
	// Proves the binary runs, the source parses and the rules load — every way
	// this fails except the doctrine values themselves.
	if _, err := c.Compile(Doctrine{Name: "startup-probe"}); err != nil {
		return nil, fmt.Errorf("vimyc cannot compile the rule set: %w", err)
	}
	return c, nil
}

// Compile runs the compiler and loads what it emits.
func (c *VimycCompiler) Compile(d Doctrine) ([]*Rule, error) {
	params, err := json.Marshal(DoctrineParams(d))
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	// vimyc takes paths, so the source goes to a temp directory; params go on
	// stdin so nothing per-doctrine touches the disk.
	srcArgs, cleanup, err := c.sources()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	args := append(append([]string{}, srcArgs...), "--params", "-", "--json")
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Stdin = bytes.NewReader(params)
	var out, errs bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errs
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("vimyc: %w: %s", err, strings.TrimSpace(errs.String()))
	}

	// Warnings — priority collisions, shadowed rules — are findings about the
	// rule set rather than failures, and are lost if nobody prints them. The
	// temp directory is stripped so they name the file as checked out.
	for _, line := range strings.Split(strings.TrimSpace(errs.String()), "\n") {
		if line != "" {
			slog.Warn("vimyc", "doctrine", d.Name, "message", trimSourceDir(line, srcArgs))
		}
	}

	return LoadArtifact(out.Bytes())
}

// sources names the rule set, with a function to release it.
//
// Individual files rather than the directory: seed.vy sits beside the doctrine
// sources and is a standalone rule set, so compiling the directory whole yields
// a set the game never runs, fingerprinted against no recording. The embedded
// path drops it too, and the two must agree.
func (c *VimycCompiler) sources() ([]string, func(), error) {
	if c.RulesDir != "" {
		found, err := filepath.Glob(filepath.Join(c.RulesDir, "*.vy"))
		if err != nil {
			return nil, func() {}, fmt.Errorf("%s: %w", c.RulesDir, err)
		}
		out := make([]string, 0, len(found))
		for _, f := range found {
			if filepath.Base(f) != "seed.vy" {
				out = append(out, f)
			}
		}
		if len(out) == 0 {
			return nil, func() {}, fmt.Errorf("%s: no .vy sources", c.RulesDir)
		}
		sort.Strings(out)
		return out, func() {}, nil
	}
	dir, err := writeRuleSet()
	if err != nil {
		return nil, func() {}, err
	}
	return []string{dir}, func() { os.RemoveAll(dir) }, nil
}

// writeRuleSet unpacks the embedded sources into a temp directory for the
// caller to remove. Handed to vimyc whole, so membership of the rule set is
// decided by what is embedded and adding a topic file needs no change here.
func writeRuleSet() (string, error) {
	entries, err := doctrineSource.ReadDir("vy")
	if err != nil {
		return "", fmt.Errorf("read rule set: %w", err)
	}
	dir, err := os.MkdirTemp("", "vimy-rules-")
	if err != nil {
		return "", fmt.Errorf("temp dir: %w", err)
	}
	written := 0
	for _, e := range entries {
		// The seed rules are a separate rule set and would collide.
		if e.IsDir() || e.Name() == "seed.vy" {
			continue
		}
		b, err := doctrineSource.ReadFile(filepath.Join("vy", e.Name()))
		if err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("read %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o600); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("write %s: %w", e.Name(), err)
		}
		written++
	}
	// vimyc would report an empty directory as a broken checkout, not a broken
	// embed.
	if written == 0 {
		os.RemoveAll(dir)
		return "", fmt.Errorf("rule set is empty: nothing embedded under rules/vy")
	}
	return dir, nil
}

// trimSourceDir rewrites a diagnostic to name the file as checked out, not the
// temp path that has since been removed.
func trimSourceDir(line string, srcArgs []string) string {
	for _, a := range srcArgs {
		if dir := filepath.Dir(a); dir != "." {
			line = strings.ReplaceAll(line, dir+"/", "")
		}
	}
	return line
}
