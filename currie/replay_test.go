package main

import "testing"

func rule(name string, seen, held, preempted, blocked int, clauses ...clauseReport) ruleReport {
	return ruleReport{
		Rule: name, Seen: seen, Held: held, Preempted: preempted,
		Blocked: blocked, Clauses: clauses,
	}
}

func clause(i, blocked, sole int, src string) clauseReport {
	return clauseReport{Index: i, Blocked: blocked, Sole: sole, Source: src, File: "t.vy", Line: i + 1}
}

// Counts from every doctrine window add up into one record per rule.
func TestMergerSumsWindows(t *testing.T) {
	m := newMerger()
	m.add(report{Rules: []ruleReport{
		rule("produce-infantry", 10, 2, 1, 7, clause(0, 7, 4, "cash >= 100"), clause(1, 3, 0, "has-role(barracks)")),
	}})
	m.add(report{Rules: []ruleReport{
		rule("produce-infantry", 5, 1, 0, 4, clause(0, 4, 3, "cash >= 100"), clause(1, 1, 1, "has-role(barracks)")),
	}})

	out := m.result(15)
	if len(out.Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(out.Rules))
	}
	r := out.Rules[0]
	if r.Seen != 15 || r.Held != 3 || r.Preempted != 1 || r.Blocked != 11 {
		t.Errorf("seen/held/preempted/blocked = %d/%d/%d/%d, want 15/3/1/11",
			r.Seen, r.Held, r.Preempted, r.Blocked)
	}
	if r.Clauses[0].Sole != 7 || r.Clauses[1].Sole != 1 {
		t.Errorf("clause sole = %d/%d, want 7/1", r.Clauses[0].Sole, r.Clauses[1].Sole)
	}
}

// The culprit is recomputed after merging: whichever clause was worst overall,
// not whichever was worst in the window that happened to be added first.
func TestMergerRecomputesTheCulprit(t *testing.T) {
	m := newMerger()
	// Window one: clause 0 is the culprit.
	m.add(report{Rules: []ruleReport{
		rule("r", 10, 0, 0, 10, clause(0, 9, 9, "cash >= 100"), clause(1, 2, 1, "has-role(barracks)")),
	}})
	// Window two: clause 1 dominates, and over both windows it wins.
	m.add(report{Rules: []ruleReport{
		rule("r", 40, 0, 0, 40, clause(0, 5, 2, "cash >= 100"), clause(1, 39, 30, "has-role(barracks)")),
	}})

	out := m.result(50)
	r := out.Rules[0]
	if r.Culprit == nil {
		t.Fatal("no culprit after merging")
	}
	if *r.Culprit != 1 {
		t.Errorf("culprit = clause %d, want 1 — clause 1 has 31 sole against clause 0's 11", *r.Culprit)
	}
}

// A rule that some doctrines dropped still merges: specialisation removes rules
// a doctrine cannot use, so windows rarely emit the same list.
func TestMergerHandlesRulesMissingFromSomeWindows(t *testing.T) {
	m := newMerger()
	m.add(report{Rules: []ruleReport{
		rule("only-in-first", 5, 0, 0, 5, clause(0, 5, 5, "naval-weight > 0.1")),
	}})
	m.add(report{Rules: []ruleReport{
		rule("only-in-second", 5, 0, 0, 5, clause(0, 5, 5, "air-weight > 0.1")),
	}})

	out := m.result(10)
	if len(out.Rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(out.Rules))
	}
	for _, r := range out.Rules {
		if r.Seen != 5 {
			t.Errorf("%s seen = %d, want 5 — it only existed in one window", r.Rule, r.Seen)
		}
	}
}

// Nothing was ever solely responsible, so there is no single edit to recommend.
func TestMergerLeavesNoCulpritWhenNothingIsSolelyToBlame(t *testing.T) {
	m := newMerger()
	m.add(report{Rules: []ruleReport{
		rule("r", 10, 0, 0, 10, clause(0, 10, 0, "a"), clause(1, 10, 0, "b")),
	}})
	if c := m.result(10).Rules[0].Culprit; c != nil {
		t.Errorf("culprit = %d, want none", *c)
	}
}
