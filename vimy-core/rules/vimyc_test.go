package rules

import (
	"os/exec"
	"testing"
)

// vimyc compiles the doctrines the game will hand it.
//
// This was a differential against CompileDoctrine, which is deleted — the port
// is verified in vimyc against a corpus that compiler generated, and keeping a
// second implementation alive to check the first is the cost that motivated
// deleting it. What remains testable here is Go's half: that every archived
// doctrine survives DoctrineParams, the subprocess and LoadArtifact, and comes
// back as rules the engine accepts.
func TestVimycCompilesEveryArchivedDoctrine(t *testing.T) {
	if _, err := exec.LookPath("vimyc"); err != nil {
		t.Skip("vimyc not on PATH")
	}
	c, err := NewVimycCompiler("")
	if err != nil {
		t.Fatalf("compiler: %v", err)
	}
	doctrines, err := RealDoctrines()
	if err != nil {
		t.Fatal(err)
	}

	// A subprocess each, so a sample. Spread across the archive rather than
	// the first N, since doctrines cluster by the game they came from.
	const step = 40
	compiled, rulesSeen := 0, 0
	for i := 0; i < len(doctrines); i += step {
		d := doctrines[i]
		rs, err := c.Compile(d)
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		if len(rs) == 0 {
			t.Fatalf("%s: no rules", d.Name)
		}
		// The engine compiles every condition and rejects an unknown one, so
		// this covers the emitted expr as well as the loader.
		if _, err := NewEngine(rs); err != nil {
			t.Fatalf("%s: engine: %v", d.Name, err)
		}
		compiled++
		rulesSeen += len(rs)
	}
	t.Logf("%d doctrines, %d rules compiled and loaded", compiled, rulesSeen)
}
