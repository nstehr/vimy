// Package vimyc owns source selection and cancellable compiler execution for
// the bot and its offline analysis tools.
package vimyc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DefaultTimeout = 10 * time.Second

type Client struct {
	Bin     string
	Timeout time.Duration
}

// Run returns diagnostics separately from stdout so callers can expose compiler
// warnings without mixing them into the JSON protocol.
func (c Client) Run(ctx context.Context, args []string, input []byte) ([]byte, []string, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	bin := c.Bin
	if bin == "" {
		bin = "vimyc"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = bytes.NewReader(input)
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, nil, fmt.Errorf("vimyc: %w: %s", err, strings.TrimSpace(errs.String()))
	}
	var diagnostics []string
	for _, line := range strings.Split(strings.TrimSpace(errs.String()), "\n") {
		if line != "" {
			diagnostics = append(diagnostics, TrimSourcePath(line, args))
		}
	}
	return out.Bytes(), diagnostics, nil
}

// Sources selects the doctrine sources. The standalone seed program is never
// part of the doctrine compilation unit.
func Sources(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.vy"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range paths {
		if filepath.Base(p) != "seed.vy" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no .vy sources", dir)
	}
	sort.Strings(out)
	return out, nil
}

func SourceDigest(paths []string) (string, error) {
	paths = append([]string(nil), paths...)
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.Base(p), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BinaryDigest identifies the actual executable, including unversioned local
// builds. Callers may cache it while the executable's file identity is stable.
func BinaryDigest(bin string) (string, error) {
	if bin == "" {
		bin = "vimyc"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func TrimSourcePath(line string, args []string) string {
	for _, a := range args {
		if !strings.HasSuffix(a, ".vy") {
			continue
		}
		if dir := filepath.Dir(a); dir != "." {
			line = strings.ReplaceAll(line, dir+string(filepath.Separator), "")
		}
	}
	return line
}
