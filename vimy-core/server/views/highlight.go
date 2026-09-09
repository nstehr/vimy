package views

import (
	"regexp"
	"strings"
)

// Syntax highlighting for `.vy` in the rules panel.
//
// Prism from a CDN, as Tailwind, htmx and Chart.js already arrive — no build
// step. The vocabulary is generated from `vimyc --tokens` into vy_tokens.go, so
// a new keyword needs no edit here, and a test fails when that file goes stale.

// VyPrismGrammar is the Prism language definition, as JavaScript.
//
// Order matters twice. Strings first, so a keyword inside a `because` stays
// prose. Identifiers before operators, because they are kebab: the identifier
// pattern takes a `-` only when a letter follows — the lexer's own rule — so
// `build-power` stays one name rather than a subtraction.
func VyPrismGrammar() string {
	return `Prism.languages.vy = {
  'string': { pattern: /"[^"]*"/, greedy: true },
  'keyword': new RegExp('\\b(?:' + ` + jsArray(vyKeywords) + `.join('|') + ')\\b'),
  'boolean': /\b(?:true|false)\b/,
  'number': /\b\d+(?:\.\d+)?\b/,
  'function': /\b[a-z][a-z0-9]*(?:-[a-z][a-z0-9]*)*(?=\()/,
  'identifier': /\b[a-z][a-z0-9]*(?:-[a-z][a-z0-9]*)*\b/,
  'operator': ` + alternation(vyOperators) + `,
  'punctuation': ` + alternation(vyPunctuation) + `
};`
}

func jsArray(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = "'" + s + "'"
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// alternation builds a JavaScript regex literal matching the spellings in the
// order given, so a longer operator is tried before the shorter one it starts
// with.
//
// Two escapes, not one: QuoteMeta covers regex metacharacters, and `/` needs
// handling on top. It means nothing to a regex but closes a JavaScript regex
// literal, so the division operator would end the pattern early and leave the
// whole definition unparseable.
func alternation(items []string) string {
	escaped := make([]string, len(items))
	for i, s := range items {
		escaped[i] = strings.ReplaceAll(regexp.QuoteMeta(s), "/", `\/`)
	}
	return "/" + strings.Join(escaped, "|") + "/"
}
