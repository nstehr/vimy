package views

import (
	"regexp"
	"strings"
)

// Syntax highlighting for `.vy` in the rules panel.
//
// Prism from a CDN, matching how Tailwind, htmx and Chart.js already arrive —
// no build step. The language definition is generated from the token tables
// below rather than written out, and `vimyc --tokens` prints the compiler's own
// copies of them so a test can compare: a token added to the language cannot
// quietly stop being highlighted.

// The language's vocabulary, mirroring `TokenKind::{KEYWORDS,OPERATORS,
// PUNCTUATION}`. Operators are longest-first, which a regex alternation
// requires — otherwise `<` matches before `<=` and the `=` is left over.
var (
	vyKeywords    = strings.Fields("rule priority category exclusive do require because let and or not exists param def int float")
	vyOperators   = strings.Fields("<= >= == != < > + - * / =")
	vyPunctuation = strings.Fields("{ } ( ) , :")
)

// VyPrismGrammar is the Prism language definition, as JavaScript.
//
// Order is significant twice over. Strings come first, so a keyword inside a
// `because` stays prose. Identifiers come last, because a kebab name would
// otherwise swallow the `-` that separates it.
func VyPrismGrammar() string {
	return `Prism.languages.vy = {
  'string': { pattern: /"[^"]*"/, greedy: true },
  'keyword': new RegExp('\\b(?:' + ` + jsArray(vyKeywords) + `.join('|') + ')\\b'),
  'boolean': /\b(?:true|false)\b/,
  'number': /\b\d+(?:\.\d+)?\b/,
  'function': /\b[a-z][a-z0-9]*(?:-[a-z][a-z0-9]*)*(?=\()/,
  'operator': ` + alternation(vyOperators) + `,
  'punctuation': ` + alternation(vyPunctuation) + `,
  'identifier': /\b[a-z][a-z0-9]*(?:-[a-z][a-z0-9]*)*\b/
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
