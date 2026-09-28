package rules

import (
	"log/slog"
	"slices"
)

// StrategicSignals are observations supplied by the strategist, independent
// of whether its next doctrine can be compiled.
type StrategicSignals struct {
	BurnedAxes        []string
	BeingRushed       bool
	HarvesterHarassed bool
}

// DoctrinePolicy changes only when the associated rules become active.
type DoctrinePolicy struct {
	RepairBudgetRatio  float64
	ScoutReachPriority float64
}

func (e *Engine) SetStrategicSignals(signals StrategicSignals) {
	signals.BurnedAxes = slices.Clone(signals.BurnedAxes)
	e.mu.Lock()
	e.signals = signals
	e.mu.Unlock()
}

// PreparedDoctrine owns compiled rules and their settings. Preparation has no
// effects on a running engine; activation cannot fail halfway through.
type PreparedDoctrine struct {
	rules  []*Rule
	record RuleSetRecord
	prefs  UnitPreferences
	bias   TargetBias
	policy DoctrinePolicy
}

func PrepareDoctrine(d Doctrine, rules []*Rule) (*PreparedDoctrine, error) {
	compiled, err := compileRules(rules)
	if err != nil {
		return nil, err
	}
	return &PreparedDoctrine{
		rules:  compiled,
		record: RuleSetRecord{Artifact: artifactFor(compiled), Doctrine: &d},
		prefs: UnitPreferences{Infantry: slices.Clone(d.PreferredInfantry), Vehicle: slices.Clone(d.PreferredVehicle),
			Aircraft: slices.Clone(d.PreferredAircraft), Naval: slices.Clone(d.PreferredNaval)},
		bias:   ComputeTargetBias(d),
		policy: DoctrinePolicy{RepairBudgetRatio: d.RepairBudgetRatio, ScoutReachPriority: d.ScoutReachPriority},
	}, nil
}

type Activation struct {
	PreviousStats map[string]RuleFiringStats
	RuleNames     []string
	Tracing       bool
}

// Activate closes the old firing window and installs every setting between
// evaluations. Callers serialize their corresponding history update.
func (e *Engine) Activate(prepared *PreparedDoctrine) Activation {
	e.memMu.Lock()
	defer e.memMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	result := Activation{Tracing: e.TracingEnabled(), PreviousStats: e.FlushFiringStats()}
	e.prefs, e.bias, e.policy = prepared.prefs, prepared.bias, prepared.policy
	e.installRules(prepared.rules)
	e.record = prepared.record
	e.exporter.RecordRuleSet(e.ruleSetID, e.record)
	for _, rule := range prepared.rules {
		result.RuleNames = append(result.RuleNames, rule.Name)
	}
	return result
}

// installRules requires memMu and mu, in that order.
func (e *Engine) installRules(compiled []*Rule) {
	e.rules, e.ruleSetID = compiled, RuleSetID(compiled)
	e.record = RuleSetRecord{Artifact: artifactFor(compiled)}
	kept, dropped := e.retainSquadsLocked(squadNames(compiled))
	slog.Info("rule set swapped", "count", len(compiled), "kept_squads", kept, "dropped_squads", dropped)
}

// PrepareCompilation carries the exact compiler inputs into the active export.
func PrepareCompilation(d Doctrine, c *Compilation) (*PreparedDoctrine, error) {
	p, err := PrepareDoctrine(d, c.Rules)
	if err != nil {
		return nil, err
	}
	p.record = c.Record
	return p, nil
}
