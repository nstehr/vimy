package rules

import (
	"reflect"
	"runtime/debug"
)

// What ran.
//
// Tuning the rule engine means asking "did that change move the number", and
// the archive could not answer it: 147 games and nothing recording which rule
// sources produced any of them. Spread sat near 20 cells against a required 8
// through four separate fixes, each judged by eye against the next game or
// two, because there was no way to group games by the code that played them.
//
// Two identifiers, because they answer different questions:
//
//	RulesDigest  did the RULES change? Moves when the `.vy` sources or vimyc's
//	             codegen move. Does NOT move when the LLM picks different
//	             doctrine weights -- RuleSetID on a live rule set does, which
//	             is why that one cannot be used for this.
//	Revision     did the SIDECAR change? Everything outside the rules: squad
//	             handling, targeting, the IPC layer.
//
// A refactor that changes no behaviour shows a new Revision and the same
// RulesDigest, which is exactly the answer wanted when asking whether a
// metric moved because of the edit.
type SourceVersion struct {
	RulesDigest string
	Revision    string
	// Modified means the binary was built from a dirty tree. A tuning run on
	// uncommitted edits is not reproducible, and a before/after that quietly
	// spans one is worse than no answer.
	Modified bool
}

// referenceDoctrines are the fixed inputs the digest is taken against. They
// must never change, or every digest changes with them.
//
// Two, not one, because a doctrine only emits the rules its numbers enable.
// Measured: an all-zero doctrine compiles to 44 rules and an all-max one to
// 110, union 112. A digest taken against all-zero alone would therefore be
// blind to 68 rules -- flee-harvesters, build-aa-defense, produce-extra-
// harvester, scramble-to-harvesters, squad-focus-fire among them, which is
// most of what anyone would actually tune. It would sit unchanged through a
// rule edit and report that nothing had happened.
//
// Both ends also catch condition changes rather than just rule presence: a
// doctrine's numbers are interpolated into the conditions, so the same rule
// compiles differently at each end and both forms enter the hash.
//
// Residual gap, stated rather than papered over: a change that only manifests
// at mid-range parameter values still slips through. Parameters are lerped
// monotonically so that is rare, but it is not impossible, and the digest is
// evidence rather than proof.
func referenceDoctrines() []Doctrine {
	maxed := Doctrine{Name: "digest-reference-max"}
	v := reflect.ValueOf(&maxed).Elem()
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.Float64 && f.CanSet() {
			f.SetFloat(1)
		}
	}
	return []Doctrine{{Name: "digest-reference"}, maxed}
}

// SourceVersion fingerprints what this compiler will produce, alongside the
// build it is running in.
//
// Called once at startup, costing two compiles of a few milliseconds each. It
// compiles reference doctrines rather than hashing the `.vy` files directly:
// what matters is the rule set that comes out, so a change in vimyc that
// alters codegen counts, and a comment added to a `.vy` file does not.
func (c *VimycCompiler) SourceVersion() (SourceVersion, error) {
	v := SourceVersion{}
	v.Revision, v.Modified = buildRevision()

	var all []*Rule
	for _, d := range referenceDoctrines() {
		rs, err := c.Compile(d)
		if err != nil {
			return v, err
		}
		all = append(all, rs...)
	}
	// RuleSetID sorts and hashes a line per rule, so the same rule compiled at
	// both ends contributes both forms and neither is lost to the other.
	v.RulesDigest = RuleSetID(all)
	return v, nil
}

// buildRevision reads the VCS stamp the Go toolchain embeds when building from
// a repository. Empty under `go run` and in tests, which is honest: those are
// not builds anyone can go back to.
func buildRevision() (rev string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 12 {
				rev = rev[:12]
			}
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return rev, modified
}
