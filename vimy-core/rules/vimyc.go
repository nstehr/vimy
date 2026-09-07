package rules

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Compiling a doctrine into rules.
//
// A subprocess rather than a library: the boundary is a doctrine in and a rule
// set out, it runs once per doctrine window rather than per tick, and a process
// cannot take the bot down with it.

// The rule set source, carried in the binary so a game needs only the compiler
// on PATH. Written by hand in `rules/vy/` — it is Vimy's strategy, the way the
// `.go` files are Vimy's code; vimyc is the language it is written in.
//
//go:embed vy/doctrine.vy
var doctrineSource []byte

// VimycCompiler turns a Doctrine into rules by running the vimyc binary.
type VimycCompiler struct {
	// Path to the binary; "vimyc" finds it on PATH.
	Bin string
	// How long to wait. Compiling 118 rules takes single-digit milliseconds,
	// so this only ever catches something being wrong.
	Timeout time.Duration
}

// NewVimycCompiler writes the embedded rule set to a file vimyc can read and
// returns a compiler for it.
//
// Checked at startup rather than at the first doctrine: a missing binary
// discovered twenty minutes into a game is a wasted game.
func NewVimycCompiler(bin string) (*VimycCompiler, error) {
	if bin == "" {
		bin = "vimyc"
	}
	c := &VimycCompiler{Bin: bin, Timeout: 10 * time.Second}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("vimyc not found: %w", err)
	}
	// Compiling a doctrine now proves the binary runs, the source parses and
	// every rule it emits can be loaded — all the ways this can fail except
	// the values themselves.
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

	// The source goes to a temp file because vimyc takes a path; the params go
	// on stdin so nothing per-doctrine touches the disk.
	src, err := os.CreateTemp("", "vimy-*.vy")
	if err != nil {
		return nil, fmt.Errorf("temp file: %w", err)
	}
	defer os.Remove(src.Name())
	if _, err := src.Write(doctrineSource); err != nil {
		return nil, fmt.Errorf("write rule set: %w", err)
	}
	if err := src.Close(); err != nil {
		return nil, fmt.Errorf("write rule set: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Bin, src.Name(), "--params", "-", "--json")
	cmd.Stdin = bytes.NewReader(params)
	var out, errs bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errs
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("vimyc: %w: %s", err, strings.TrimSpace(errs.String()))
	}

	// Warnings — priority collisions, shadowed rules — are findings about the
	// rule set rather than failures, and are lost if nobody prints them.
	for _, line := range strings.Split(strings.TrimSpace(errs.String()), "\n") {
		if line != "" {
			slog.Warn("vimyc", "doctrine", d.Name, "message", line)
		}
	}

	return LoadArtifact(out.Bytes())
}
