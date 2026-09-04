package views

import (
	"os/exec"
	"strings"
	"testing"
)

// The highlighter's vocabulary is the language's vocabulary.
//
// A token added to vimyc and not here would still compile and would silently
// render as plain text — the kind of drift nobody notices until they are
// staring at a rule wondering why one word looks wrong. `vimyc --tokens` prints
// the compiler's own tables for exactly this.
func TestVyTokensMatchTheCompiler(t *testing.T) {
	// Skip only when the binary is absent. A binary that is present and fails
	// is a stale one, and treating that as "not installed" makes the test
	// disappear exactly when it would have caught something.
	if _, err := exec.LookPath("vimyc"); err != nil {
		t.Skip("vimyc not on PATH")
	}
	out, err := exec.Command("vimyc", "--tokens").Output()
	if err != nil {
		t.Fatalf("vimyc --tokens failed, is the binary stale? %v", err)
	}

	theirs := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Fatalf("cannot read %q", line)
		}
		theirs[fields[0]] = fields[1:]
	}

	for _, c := range []struct {
		kind string
		mine []string
	}{
		{"keyword", vyKeywords},
		{"operator", vyOperators},
		{"punctuation", vyPunctuation},
	} {
		got, ok := theirs[c.kind]
		if !ok {
			t.Errorf("vimyc printed no %s section", c.kind)
			continue
		}
		if strings.Join(got, " ") != strings.Join(c.mine, " ") {
			// Order matters for operators, so this compares sequences rather
			// than sets: `<` before `<=` would highlight half an operator.
			t.Errorf("%s differs\n  vimyc: %v\n  here:  %v", c.kind, got, c.mine)
		}
	}
}

// The generated grammar is JavaScript that mentions the whole vocabulary.
func TestVyPrismGrammarIsWellFormed(t *testing.T) {
	js := VyPrismGrammar()
	for _, k := range vyKeywords {
		if !strings.Contains(js, "'"+k+"'") {
			t.Errorf("keyword `%s` is missing", k)
		}
	}
	if strings.Count(js, "{") != strings.Count(js, "}") {
		t.Error("unbalanced braces")
	}

	// Strings before keywords, so a keyword inside a `because` stays prose.
	if strings.Index(js, "'string'") > strings.Index(js, "'keyword'") {
		t.Error("strings must be matched before keywords")
	}
	// Identifiers last, or a kebab name swallows the operators.
	if strings.Index(js, "'identifier'") < strings.Index(js, "'operator'") {
		t.Error("identifiers must be matched after operators")
	}
	// Every regex literal must be escaped for two contexts: regex
	// metacharacters, and the `/` that would close the literal early. Getting
	// the second wrong is silent — the definition fails to parse and the page
	// simply has no highlighting.
	for _, line := range strings.Split(js, "\n") {
		body, ok := regexLiteral(line)
		if !ok {
			continue
		}
		for i, c := range body {
			if c != '/' {
				continue
			}
			if i == 0 || body[i-1] != '\\' {
				t.Errorf("unescaped `/` closes the literal early: %s", line)
			}
		}
	}
	if !strings.Contains(js, `\*`) || !strings.Contains(js, `\+`) {
		t.Errorf("operator alternation is not escaped: %s", js)
	}
}

// regexLiteral returns the body of a `/.../` on a line of the generated
// definition, if there is one.
func regexLiteral(line string) (string, bool) {
	line = strings.TrimSpace(line)
	i := strings.Index(line, ": /")
	if i < 0 {
		return "", false
	}
	body := line[i+3:]
	j := strings.LastIndex(body, "/")
	if j < 0 {
		return "", false
	}
	return body[:j], true
}
