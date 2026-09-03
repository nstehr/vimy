package rules

import (
	"strconv"
	"strings"
)

// actionSrc renders a parameterised action the way vimyc spells it.
//
// Called alongside the factory at each site rather than derived from the
// resulting closure, because closures share a code pointer and their captured
// arguments cannot be read back.
func actionSrc(name string, args ...any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case string:
			parts[i] = vimycLiteral(v)
		case int:
			parts[i] = strconv.Itoa(v)
		case float64:
			parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			parts[i] = "?"
		}
	}
	return name + "(" + strings.Join(parts, ", ") + ")"
}

// vimycLiteral maps a Go argument to the language's spelling: squad domains and
// roles are capitalised there, roles are kebab.
func vimycLiteral(v string) string {
	switch v {
	case "ground", "air", "naval", "attack", "defend":
		return strings.ToUpper(v[:1]) + v[1:]
	}
	return strings.ReplaceAll(v, "_", "-")
}
