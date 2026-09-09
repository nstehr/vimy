package rules

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// Translating compiled rules into vimyc source.
//
// Mechanical rather than by hand: a doctrine emits around 80 rules from 117
// distinct names, and a hand translation would have to be redone whenever the
// compiler changes. The differential test checks it stays faithful.

// ToVimyc renders a compiled rule set as a .vy file.
func ToVimyc(rules []*Rule) (string, error) {
	var b strings.Builder
	for i, r := range rules {
		if i > 0 {
			b.WriteString("\n")
		}
		s, err := ruleToVimyc(r)
		if err != nil {
			return "", fmt.Errorf("rule %q: %w", r.Name, err)
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

func ruleToVimyc(r *Rule) (string, error) {
	action, err := actionName(r)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "rule %s {\n", r.Name)
	fmt.Fprintf(&b, "  priority %d\n", r.Priority)
	excl := ""
	if r.Exclusive {
		excl = " exclusive"
	}
	fmt.Fprintf(&b, "  category %s%s\n", kebab(r.Category), excl)
	fmt.Fprintf(&b, "  do       %s\n\n", action)

	for _, c := range splitConjuncts(r.ConditionSrc) {
		t, err := translateExpr(c)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "  require %s\n", t)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// splitConjuncts breaks a condition on `&&` at paren depth zero, one `require`
// each — conjunction is structural in the language.
func splitConjuncts(cond string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(cond); i++ {
		switch cond[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '&':
			if depth == 0 && i+1 < len(cond) && cond[i+1] == '&' {
				out = append(out, strings.TrimSpace(cond[start:i]))
				i++
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(cond[start:]))

	var kept []string
	for _, c := range out {
		if c = strings.TrimSpace(c); c != "" {
			kept = append(kept, unwrap(c))
		}
	}
	return kept
}

// unwrap drops parens around a whole conjunct; each `require` is already a unit.
func unwrap(s string) string {
	for len(s) > 1 && s[0] == '(' && s[len(s)-1] == ')' {
		depth := 0
		for i := 0; i < len(s); i++ {
			if s[i] == '(' {
				depth++
			} else if s[i] == ')' {
				depth--
				if depth == 0 && i != len(s)-1 {
					return s // the parens are not a single wrapper
				}
			}
		}
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

var (
	reCall    = regexp.MustCompile(`([A-Z]\w*)\(([^()]*)\)`)
	reLen     = regexp.MustCompile(`\blen\(`)
	reNotNil  = regexp.MustCompile(`([a-z][\w-]*(?:\([^()]*\))?)\s*!=\s*nil`)
	reEqNil   = regexp.MustCompile(`([a-z][\w-]*(?:\([^()]*\))?)\s*==\s*nil`)
	reBareArg = regexp.MustCompile(`"([^"]*)"`)
)

// translateExpr rewrites one expr conjunct into vimyc syntax: calls, literals,
// boolean operators, comparisons and arithmetic. The corpus has no ternaries,
// indexing or field access, and anything unexpected errors rather than passing
// through silently.
func translateExpr(s string) (string, error) {
	// `len(X(...))` counts a collection.
	for reLen.MatchString(s) {
		i := reLen.FindStringIndex(s)
		j, depth := i[1]-1, 0
		for ; j < len(s); j++ {
			if s[j] == '(' {
				depth++
			} else if s[j] == ')' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		inner := s[i[1]:j]
		s = s[:i[0]] + "count(" + inner + ")" + s[j+1:]
	}

	// Calls: PascalCase to kebab, and drop the parens when there are no args.
	s = reCall.ReplaceAllStringFunc(s, func(m string) string {
		p := reCall.FindStringSubmatch(m)
		name, args := goKebab(p[1]), strings.TrimSpace(p[2])
		if args == "" {
			return name
		}
		return name + "(" + args + ")"
	})

	// Optionals: `nearest-enemy != nil` becomes `exists nearest-enemy`.
	s = reNotNil.ReplaceAllString(s, "exists $1")
	s = reEqNil.ReplaceAllString(s, "not exists $1")

	// String literals become bare enum names; roles are snake in Go.
	s = reBareArg.ReplaceAllStringFunc(s, func(m string) string {
		v := m[1 : len(m)-1]
		return kebab(v)
	})

	// A count of a type, not of a collection.
	s = strings.ReplaceAll(s, "building-count(", "count(")
	s = strings.ReplaceAll(s, "unit-count(", "count(")

	s = strings.ReplaceAll(s, "&&", "and")
	s = strings.ReplaceAll(s, "||", "or")
	s = regexp.MustCompile(`!\s*`).ReplaceAllString(s, "not ")

	s = strings.Join(strings.Fields(s), " ")
	if strings.ContainsAny(s, `"?[]`) {
		return "", fmt.Errorf("untranslated construct in %q", s)
	}
	return s, nil
}

// actionName renders a rule's action, with arguments for factory-built ones.
// Registry lookups go by function pointer — a Rule holds the function, not the id.
func actionName(r *Rule) (string, error) {
	ptr := reflect.ValueOf(r.Action).Pointer()
	for id, fn := range ActionRegistry {
		if reflect.ValueOf(fn).Pointer() == ptr {
			return id, nil
		}
	}
	if r.ActionSrc != "" {
		return r.ActionSrc, nil
	}
	return "", fmt.Errorf("action cannot be named (no registry id and no ActionSrc)")
}
