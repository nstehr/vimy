package views

import "strings"

// Syntax highlighting for `.vy` in the rules panel.
//
// Prism from a CDN, matching how Tailwind, htmx and Chart.js already arrive —
// no build step. The language definition is generated from `vyKeywords` rather
// than written out, so adding a keyword to the language is the only edit needed.

// vyKeywords is the language's keyword list, and must match vimyc's.
//
// `TestVyKeywordsMatchTheCompiler` runs `vimyc --keywords` and compares, so a
// keyword added to the language cannot quietly stop being highlighted.
var vyKeywords = []string{
	"rule", "priority", "category", "exclusive", "do", "require", "because",
	"let", "and", "or", "not", "exists", "param", "def", "int", "float",
}

// VyPrismGrammar is the Prism language definition, as JavaScript.
//
// Ordered deliberately: Prism takes the first pattern that matches, so strings
// come before everything (a keyword inside a `because` is not a keyword), and
// identifiers come last because kebab names would otherwise swallow operators.
func VyPrismGrammar() string {
	return `Prism.languages.vy = {
  'string': { pattern: /"[^"]*"/, greedy: true },
  'keyword': new RegExp('\\b(?:' + ` + jsArray(vyKeywords) + `.join('|') + ')\\b'),
  'boolean': /\b(?:true|false)\b/,
  'number': /\b\d+(?:\.\d+)?\b/,
  'function': /\b[a-z][a-z0-9]*(?:-[a-z][a-z0-9]*)*(?=\()/,
  'operator': /<=|>=|==|!=|[<>+\-*\/=]/,
  'punctuation': /[{}(),:]/,
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
