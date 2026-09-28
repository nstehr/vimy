package vimyc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunnerCancellationAndDiagnostics(t *testing.T) {
	c := Client{Bin: "sh", Timeout: 50 * time.Millisecond}
	_, _, err := c.Run(t.Context(), []string{"-c", "exec sleep 10"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout=%v", err)
	}
	out, diagnostics, err := c.Run(t.Context(), []string{"-c", "echo '{\"ok\":true}'; echo 'warning: test' >&2"}, nil)
	if err != nil || len(out) == 0 || len(diagnostics) != 1 || diagnostics[0] != "warning: test" {
		t.Fatalf("%s %v %v", out, diagnostics, err)
	}
}
func TestBundleFreezesSourcesAndRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "core.vy")
	if err := os.WriteFile(file, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "seed.vy"), []byte("seed"), 0600)
	paths, err := Sources(dir)
	if err != nil || len(paths) != 1 {
		t.Fatalf("%v %v", paths, err)
	}
	bundle, err := ReadBundle(paths)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(file, []byte("new"), 0600)
	frozen, cleanup, err := bundle.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	raw, err := os.ReadFile(frozen[0])
	if err != nil || string(raw) != "old" {
		t.Fatalf("%s %v", raw, err)
	}
	before, _ := SourceDigest(frozen)
	after, _ := SourceDigest(paths)
	if before == after {
		t.Fatal("digest ignored source edit")
	}
	if _, cleanup, err := (Bundle{"../outside.vy": "bad"}).Materialize(); err == nil {
		cleanup()
		t.Fatal("accepted path outside source bundle")
	}
}
