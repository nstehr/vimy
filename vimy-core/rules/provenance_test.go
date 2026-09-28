package rules

import (
	"encoding/json"
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/vimyc"
)

func TestExportRetainsActivatedArtifactAndProvenance(t *testing.T) {
	seed, err := SeedRules()
	if err != nil {
		t.Fatal(err)
	}
	d := Doctrine{Name: "recorded"}
	record := RuleSetRecord{Artifact: artifactFor(seed), Doctrine: &d, Sources: vimyc.Bundle{"core.vy": "source"}, SourceDigest: "sources", CompilerDigest: "compiler"}
	prepared, err := PrepareCompilation(d, &Compilation{Rules: seed, Record: record})
	if err != nil {
		t.Fatal(err)
	}
	e, _ := NewEngine(nil)
	exporter, err := NewStateExporter(t.TempDir(), 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	e.SetExporter(exporter)
	e.Activate(prepared)
	_ = e.Evaluate(model.GameState{Tick: 10}, "england", nil)
	path, err := exporter.Flush()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ReadExport(path)
	if err != nil {
		t.Fatal(err)
	}
	var archive struct {
		RuleSets map[string]RuleSetRecord `json:"rule_sets"`
	}
	if err := json.Unmarshal(raw, &archive); err != nil {
		t.Fatal(err)
	}
	got := archive.RuleSets[RuleSetID(seed)]
	if got.CompilerDigest != "compiler" || got.SourceDigest != "sources" || got.Sources["core.vy"] != "source" || got.Doctrine.Name != "recorded" {
		t.Fatalf("lost provenance: %+v", got)
	}
	loaded, err := LoadArtifact(got.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	if RuleSetID(loaded) != RuleSetID(seed) {
		t.Fatal("archived artifact differs from active rules")
	}
}
