package views

import (
	"regexp"
	"strings"
)

// Syntax highlighting for `.vy` in the rules panel.
//
// Prism from a CDN, matching how Tailwind, htmx and Chart.js already arrive —
// no build step. The vocabulary comes from `vy_tokens.go`, which `make rules`
// generates from `vimyc --tokens`, so adding a keyword or an operator to the
// language needs no edit here. `TestVyTokensMatchTheCompiler` fails when the
// generated file is stale.

// VyPrismGrammar is the Prism language definition, as JavaScript.
//
// Order is significant twice over. Strings come first, so a keyword inside a
// `because` stays prose. Identifiers come before operators, because they are
// kebab: the identifier pattern consumes a `-` only when a letter follows, which
// is the lexer's own rule, so `build-power` stays one name instead of splitting
// into two around a subtraction.
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

// alternation builds a JavaScript regex literal matching any of the spellings,
// in the order given — so a longer operator is tried before the shorter one it
// starts with.
//
// Two escapes, not one. `QuoteMeta` handles the regex metacharacters, and `/`
// needs handling on top: it is not special to a regex but it closes a
// JavaScript regex literal, so the division operator would end the pattern
// early and leave the rest as a syntax error — with the whole definition
// failing to parse and nothing highlighting at all.
func alternation(items []string) string {
	escaped := make([]string, len(items))
	for i, s := range items {
		escaped[i] = strings.ReplaceAll(regexp.QuoteMeta(s), "/", `\/`)
	}
	return "/" + strings.Join(escaped, "|") + "/"
}
