package main

import "testing"

// An existence test is recognised whatever shape it takes.
func TestThingOfRecognisesExistenceTests(t *testing.T) {
	for clause, want := range map[string]string{
		"require count(idle-minelayers) > 0":         "minelayer",
		"require count(idle-engineers) > 0":          "engineer",
		"require role-count(harvester) == 0":         "harvester",
		"require has-role(airfield)":                 "airfield",
		"require has-unit(mcv)":                      "mcv",
		"require count(unassigned-idle-ground) >= 1": "ground",
	} {
		if got := thingOf(clause); got != want {
			t.Errorf("thingOf(%q) = %q, want %q", clause, got, want)
		}
	}
}

// A real threshold is not an existence test and must keep its curve.
func TestThingOfIgnoresThresholds(t *testing.T) {
	for _, clause := range []string{
		"require cash >= 600",
		"require role-count(harvester) < role-count(refinery) + 1",
		"require count(e1) < 10",
		"require power-excess >= 0",
	} {
		if got := thingOf(clause); got != "" {
			t.Errorf("thingOf(%q) = %q, want no thing — it is a threshold", clause, got)
		}
	}
}

func chainFixture() (report, []deadRule) {
	blocked := 0
	rep := report{Rules: []ruleReport{
		{Rule: "lay-mines", Action: "lay-mines", Clauses: []clauseReport{
			{Index: 0, Source: "require count(idle-minelayers) > 0", File: "production.vy", Line: 365},
		}},
		{Rule: "produce-minelayer", Action: "produce-minelayer", Culprit: &blocked, Clauses: []clauseReport{
			{Index: 0, Source: "require cash >= 800", File: "production.vy", Line: 340},
		}},
	}}
	dead := []deadRule{{
		Name: "lay-mines", Acted: 0,
		Culprit: &clauseReport{Source: "require count(idle-minelayers) > 0", File: "production.vy", Line: 365},
	}}
	return rep, dead
}

// The chain names the maker and what stopped it, rather than the gate that
// merely reported the absence.
func TestChainFollowsAMissingThingToItsMaker(t *testing.T) {
	rep, dead := chainFixture()
	out := chains(rep, dead, map[string]int{"produce-minelayer": 0})
	if len(out) != 1 {
		t.Fatalf("chains = %d, want 1", len(out))
	}
	c := out[0]
	if c.Thing != "minelayer" {
		t.Errorf("thing = %q, want minelayer", c.Thing)
	}
	if c.Maker != "produce-minelayer" {
		t.Errorf("maker = %q, want produce-minelayer", c.Maker)
	}
	if c.MakerCulprit != "require cash >= 800" {
		t.Errorf("maker culprit = %q, want the cash gate that stopped it", c.MakerCulprit)
	}
	if got := c.Verdict(); got != "the rule that makes one was itself blocked" {
		t.Errorf("verdict = %q", got)
	}
}

// A rule that already fires is not chased: the sample simply missed it.
func TestChainSkipsRulesThatAlreadyWork(t *testing.T) {
	rep, dead := chainFixture()
	dead[0].Acted = 9
	if out := chains(rep, dead, nil); len(out) != 0 {
		t.Errorf("chains = %d, want none — the rule already acts", len(out))
	}
}

// Nothing produces the thing at all, which is a bigger finding than a gate.
func TestChainReportsWhenNothingMakesTheThing(t *testing.T) {
	rep, dead := chainFixture()
	rep.Rules = rep.Rules[:1] // drop produce-minelayer
	out := chains(rep, dead, nil)
	if len(out) != 1 {
		t.Fatalf("chains = %d, want 1", len(out))
	}
	if out[0].Maker != "" {
		t.Errorf("maker = %q, want none", out[0].Maker)
	}
	if got := out[0].Verdict(); got != "nothing in the rule set makes one" {
		t.Errorf("verdict = %q", got)
	}
}

// `count(unassigned-idle-ground) > 0` reduces to "ground", which nothing
// builds — reporting that no rule produces one is nonsense, not a finding.
func TestChainIgnoresNounsNothingBuilds(t *testing.T) {
	rep := report{Rules: []ruleReport{
		{Rule: "scramble-base-defense", Action: "defend-base", Clauses: []clauseReport{
			{Index: 0, Source: "require count(unassigned-idle-ground) > 0"},
		}},
	}}
	dead := []deadRule{{
		Name:    "scramble-base-defense",
		Culprit: &clauseReport{Source: "require count(unassigned-idle-ground) > 0"},
	}}
	if out := chains(rep, dead, nil); len(out) != 0 {
		t.Errorf("chains = %+v, want none — \"ground\" is not a thing rules build", out)
	}
}

// A maker whose only blocker is "the thing already exists" has succeeded, and
// following it reports a completed deployment as a failure.
func TestChainSkipsAMakerThatAlreadySucceeded(t *testing.T) {
	blocked := 0
	rep := report{Rules: []ruleReport{
		{Rule: "deploy-mcv", Action: "deploy-mcv", Clauses: []clauseReport{
			{Index: 0, Source: "require has-unit(mcv)"},
		}},
		{Rule: "recover-mcv", Action: "produce-mcv", Culprit: &blocked, Clauses: []clauseReport{
			{Index: 0, Source: "require not has-role(construction-yard)"},
		}},
	}}
	dead := []deadRule{{
		Name:    "deploy-mcv",
		Culprit: &clauseReport{Source: "require has-unit(mcv)"},
	}}
	if out := chains(rep, dead, nil); len(out) != 0 {
		t.Errorf("chains = %+v, want none — the MCV was deployed, which is success", out)
	}
}
