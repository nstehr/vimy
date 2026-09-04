package rules

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Loading a rule set compiled by vimyc.
//
// The conditions are expr, so `compileRules` handles them unchanged — that is
// the whole reason the expr backend exists. Only the action needs work: a Rule
// holds a function, and JSON can only carry its name.

// The compiled form of vimyc/rules/seed.vy, which is the hand translation of
// `DefaultRules`. Committed rather than built, so a normal `go build` does not
// need the Rust toolchain; regenerate with `make rules`.
//
//go:embed seed_rules.json
var seedArtifact []byte

// SeedRules is `DefaultRules` by way of vimyc.
//
// Not yet wired into the agent: `TestSeedArtifactAgreesWithDefaultRules` is what
// has to pass before it replaces anything.
func SeedRules() ([]*Rule, error) {
	return LoadArtifact(seedArtifact)
}

// artifactRule mirrors vimyc's `RuleSource`. The tags are the contract between
// the two repos.
type artifactRule struct {
	Name      string `json:"name"`
	Priority  int    `json:"priority"`
	Category  string `json:"category"`
	Exclusive bool   `json:"exclusive"`
	Action    string `json:"action"`
	Condition string `json:"condition"`
}

// LoadArtifact turns vimyc's output into rules the engine can take.
//
// Does not compile the conditions; `NewEngine` and `Swap` already do that, and
// doing it here would mean two places that could disagree about how.
func LoadArtifact(data []byte) ([]*Rule, error) {
	var loaded []artifactRule
	if err := json.Unmarshal(data, &loaded); err != nil {
		return nil, fmt.Errorf("parse artifact: %w", err)
	}

	rules := make([]*Rule, 0, len(loaded))
	for _, a := range loaded {
		action, err := resolveAction(a.Action)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", a.Name, err)
		}
		r := &Rule{
			Name:         a.Name,
			Priority:     a.Priority,
			Category:     a.Category,
			Exclusive:    a.Exclusive,
			ConditionSrc: a.Condition,
			Action:       action,
		}
		// Only a factory-built action needs this; a registry id names itself.
		if strings.ContainsRune(a.Action, '(') {
			r.ActionSrc = a.Action
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// resolveAction is the inverse of `actionName`: a registry id, or a factory
// call with its arguments.
func resolveAction(src string) (ActionFunc, error) {
	name, args, err := parseActionSrc(src)
	if err != nil {
		return nil, err
	}
	if args == nil {
		fn, ok := ActionRegistry[name]
		if !ok {
			return nil, fmt.Errorf("unknown action %q", name)
		}
		return fn, nil
	}
	build, ok := actionFactories[name]
	if !ok {
		return nil, fmt.Errorf("unknown action factory %q", name)
	}
	fn, err := build(args)
	if err != nil {
		return nil, fmt.Errorf("action %q: %w", name, err)
	}
	return fn, nil
}

// parseActionSrc splits `form-squad(ground-attack, Ground, 8, Attack)` into its
// name and arguments. A bare name yields nil arguments, which is what separates
// a registry lookup from a factory call.
//
// No quoting or nesting to handle: every argument is a name or a number, which
// `vimycLiteral` and the grammar both guarantee.
func parseActionSrc(src string) (string, []string, error) {
	src = strings.TrimSpace(src)
	open := strings.IndexRune(src, '(')
	if open < 0 {
		return src, nil, nil
	}
	if !strings.HasSuffix(src, ")") {
		return "", nil, fmt.Errorf("malformed action %q", src)
	}
	name := strings.TrimSpace(src[:open])
	inner := strings.TrimSpace(src[open+1 : len(src)-1])
	if inner == "" {
		return "", nil, fmt.Errorf("action %q has an empty argument list", src)
	}
	args := strings.Split(inner, ",")
	for i := range args {
		args[i] = strings.TrimSpace(args[i])
		if args[i] == "" {
			return "", nil, fmt.Errorf("action %q has an empty argument", src)
		}
	}
	return name, args, nil
}

// actionFactories builds the actions that take arguments.
//
// Each entry converts its own arguments rather than sharing a general inverse of
// `vimycLiteral`, because there isn't one: a squad name is kebab on both sides
// while a role is kebab there and snake here, and the string alone cannot say
// which it is. The signature can, so the conversion lives with the signature.
var actionFactories = map[string]func([]string) (ActionFunc, error){
	"form-squad": func(a []string) (ActionFunc, error) {
		if err := arity(a, 4); err != nil {
			return nil, err
		}
		size, err := argInt(a[2])
		if err != nil {
			return nil, err
		}
		return FormSquad(a[0], lower(a[1]), size, lower(a[3])), nil
	},
	"squad-attack-move":       squadAction(SquadAttackMove),
	"squad-air-strike":        squadAction(SquadAirStrike),
	"squad-focus-fire":        squadAction(SquadFocusFire),
	"squad-disengage":         squadAction(SquadDisengage),
	"squad-defend":            squadAction(SquadDefend),
	"squad-attack-known-base": squadFloatAction(SquadAttackKnownBase),
	"recall-overextended":     squadFloatAction(RecallOverextended),
	"retreat-damaged-units":   floatAction(RetreatDamagedUnits),
	"clear-healed-units":      floatAction(ClearHealedUnits),
	"flee-harvesters":         floatAction(FleeHarvesters),
}

func squadAction(build func(string) ActionFunc) func([]string) (ActionFunc, error) {
	return func(a []string) (ActionFunc, error) {
		if err := arity(a, 1); err != nil {
			return nil, err
		}
		return build(a[0]), nil
	}
}

func squadFloatAction(build func(string, float64) ActionFunc) func([]string) (ActionFunc, error) {
	return func(a []string) (ActionFunc, error) {
		if err := arity(a, 2); err != nil {
			return nil, err
		}
		f, err := argFloat(a[1])
		if err != nil {
			return nil, err
		}
		return build(a[0], f), nil
	}
}

func floatAction(build func(float64) ActionFunc) func([]string) (ActionFunc, error) {
	return func(a []string) (ActionFunc, error) {
		if err := arity(a, 1); err != nil {
			return nil, err
		}
		f, err := argFloat(a[0])
		if err != nil {
			return nil, err
		}
		return build(f), nil
	}
}

func arity(args []string, want int) error {
	if len(args) != want {
		return fmt.Errorf("expected %d arguments, got %d", want, len(args))
	}
	return nil
}

func argInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("expected an integer, got %q", s)
	}
	return n, nil
}

func argFloat(s string) (float64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("expected a number, got %q", s)
	}
	return f, nil
}

// lower undoes the capitalisation `vimycLiteral` applies to squad domains and
// roles, which the language spells `Ground` and `Attack`.
func lower(s string) string { return strings.ToLower(s) }
