package views

import (
	"os/exec"
	"strings"
	"testing"
)

// The highlighter's keywords are the language's keywords.
//
// A keyword added to vimyc and not here would still compile and would silently
// stop being highlighted — the kind of drift nobody notices until they are
// staring at a rule wondering why one word looks wrong. vimyc prints its own
// list for exactly this.
func TestVyKeywordsMatchTheCompiler(t *testing.T) {
	out, err := exec.Command("vimyc", "--keywords").Output()
	if err != nil {
		t.Skipf("vimyc not on PATH: %v", err)
	}
	theirs := strings.Fields(string(out))

	if len(theirs) != len(vyKeywords) {
		t.Fatalf("vimyc has %d keywords, the highlighter %d:\n  vimyc: %v\n  here:  %v",
			len(theirs), len(vyKeywords), theirs, vyKeywords)
	}
	mine := map[string]bool{}
	for _, k := range vyKeywords {
		mine[k] = true
	}
	for _, k := range theirs {
		if !mine[k] {
			t.Errorf("`%s` is a keyword the highlighter does not know", k)
		}
	}
}

// The generated grammar is JavaScript that mentions every keyword.
func TestVyPrismGrammarIsWellFormed(t *testing.T) {
	js := VyPrismGrammar()
	for _, k := range vyKeywords {
		if !strings.Contains(js, "'"+k+"'") {
			t.Errorf("`%s` is missing from the grammar", k)
		}
	}
	if strings.Count(js, "{") != strings.Count(js, "}") {
		t.Error("unbalanced braces")
	}
	// `because` strings must be matched before keywords, or a keyword inside a
	// rationale would be highlighted as one.
	if strings.Index(js, "'string'") > strings.Index(js, "'keyword'") {
		t.Error("strings must be matched before keywords")
	}
}
