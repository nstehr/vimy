package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
)

func TestRevisionScopesPersistentReadings(t *testing.T) {
	c, err := newCache(t.TempDir(), rulesDir(t, "rule r {}"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	first := c.forRevision("first")
	first.write(1, &Insight{Summary: "old export"})
	second := c.forRevision("second")
	if second.read(1) != nil {
		t.Fatal("served stale replay facts")
	}
	second.write(1, &Insight{Summary: "new export"})
	if first.read(1).Summary != "old export" {
		t.Fatal("revision mutated another job's cache")
	}
}

func TestReplayRejectsInvalidRecordedArtifact(t *testing.T) {
	// No real compiler is needed: the fake accepts the compile/blame protocol.
	dir := t.TempDir()
	source := filepath.Join(dir, "core.vy")
	os.WriteFile(source, []byte("rule r {}"), 0600)
	bin := filepath.Join(dir, "vimyc")
	os.WriteFile(bin, []byte("#!/bin/sh\ncase \"$*\" in\n *--json*) echo '[{\"name\":\"r\",\"priority\":1,\"category\":\"test\",\"condition\":\"true\",\"action\":\"produce-mcv\"}]';;\n *) echo '{\"states\":1,\"rules\":[]}';;\nesac\n"), 0700)
	compiler := &rules.VimycCompiler{Bin: bin, RulesDir: dir}
	compiled, err := compiler.CompileContext(t.Context(), rules.Doctrine{Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	id := rules.RuleSetID(compiled)
	raw, err := json.Marshal(exportFile{States: []json.RawMessage{json.RawMessage(`{}`)}, Cases: []exportCase{{State: 0, Tick: 10, RuleSet: id}}, RuleSets: map[string]rules.RuleSetRecord{id: {Artifact: json.RawMessage(`[]`)}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = replayInputs(t.Context(), store.ReplayableGame{ID: 1}, raw, []store.DoctrineWindow{{DoctrineJSON: `{"name":"test"}`, Tick: 1}}, nil, []string{source}, bin)
	if err == nil || !strings.Contains(err.Error(), "invalid artifact") {
		t.Fatalf("bad provenance accepted: %v", err)
	}
}

func TestBlameHonorsCancellation(t *testing.T) {
	dir := rulesDir(t, "rule r {}")
	bin := filepath.Join(t.TempDir(), "vimyc")
	os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 10\n"), 0700)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, _, err := blameStatesContext(ctx, nil, nil, dir, bin, nil)
	if err == nil || ctx.Err() == nil {
		t.Fatalf("cancellation not propagated: %v", err)
	}
}
