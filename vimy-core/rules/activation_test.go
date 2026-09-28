package rules

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nstehr/vimy/vimy-core/model"
)

func TestFailedPreparationKeepsActiveDoctrine(t *testing.T) {
	old := &Rule{Name: "old", ConditionSrc: "true", Action: func(env RuleEnv, _ CommandSender) error { markEffect(env); return nil }}
	e, err := NewEngine([]*Rule{old})
	if err != nil {
		t.Fatal(err)
	}
	e.SetTraceFirings(true)
	if err := e.Evaluate(model.GameState{Tick: 10}, "england", nil); err != nil {
		t.Fatal(err)
	}
	before := e.FiringStatsSnapshot()
	if _, err := PrepareDoctrine(Doctrine{RepairBudgetRatio: .9}, []*Rule{{Name: "bad", ConditionSrc: "not valid ("}}); err == nil {
		t.Fatal("invalid doctrine prepared")
	}
	if e.RuleNames()[0] != "old" || e.policy.RepairBudgetRatio != 0 || !reflect.DeepEqual(before, e.FiringStatsSnapshot()) {
		t.Fatal("preparation changed live state")
	}
}

func TestActivationClosesWindowAndInstallsPolicy(t *testing.T) {
	e, _ := NewEngine([]*Rule{{Name: "old", ConditionSrc: "true", Action: func(env RuleEnv, _ CommandSender) error { markEffect(env); return nil }}})
	e.SetTraceFirings(true)
	_ = e.Evaluate(model.GameState{Tick: 10}, "england", nil)
	d := Doctrine{RepairBudgetRatio: .3, ScoutReachPriority: .7, PreferredVehicle: []string{"tank"}}
	var observed RuleEnv
	prepared, err := PrepareDoctrine(d, []*Rule{{Name: "new", ConditionSrc: "true", Action: func(env RuleEnv, _ CommandSender) error { observed = env; return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	d.PreferredVehicle[0] = "changed"
	activation := e.Activate(prepared)
	if activation.PreviousStats["old"].ActCount != 1 || len(e.FiringStatsSnapshot()) != 0 {
		t.Fatal("firing window not closed")
	}
	_ = e.Evaluate(model.GameState{Tick: 20}, "england", nil)
	if observed.Policy.RepairBudgetRatio != .3 || observed.Policy.ScoutReachPriority != .7 || observed.Preferences.Vehicle[0] != "tank" {
		t.Fatalf("mixed activation: %+v", observed)
	}
	signals := StrategicSignals{BeingRushed: true, BurnedAxes: []string{"air"}}
	e.SetStrategicSignals(signals)
	signals.BurnedAxes[0] = "vehicle"
	_ = e.Evaluate(model.GameState{Tick: 30}, "england", nil)
	if !observed.IsRushed() || !observed.AxisBurned("air") {
		t.Fatal("signals alias caller memory")
	}
	e.Reset()
	if len(e.FiringStatsSnapshot()) != 0 || e.signals.BeingRushed || e.policy.RepairBudgetRatio != 0 {
		t.Fatal("reset retained game state")
	}
}

type fakeSender struct {
	sent   int
	failAt int
}

func (s *fakeSender) Send(_ string, _ any) error {
	if s.sent == s.failAt {
		return errors.New("send failed")
	}
	s.sent++
	return nil
}
func TestActionResultCountsOnlySuccessfulCommands(t *testing.T) {
	sender := &fakeSender{failAt: 1}
	result, err := RunAction(func(env RuleEnv, s CommandSender) error {
		if err := s.Send("one", nil); err != nil {
			return err
		}
		return s.Send("two", nil)
	}, RuleEnv{}, sender)
	if err == nil || result.CommandsSent != 1 || !result.Acted() {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	result, err = RunAction(func(env RuleEnv, _ CommandSender) error { markEffect(env); return nil }, RuleEnv{}, nil)
	if err != nil || !result.StateChanged {
		t.Fatal("lost memory-only effect")
	}
	result, err = RunAction(func(_ RuleEnv, _ CommandSender) error { return nil }, RuleEnv{}, nil)
	if err != nil || result.Acted() || result.NoOpReason == "" {
		t.Fatal("no-op counted as work")
	}
}

func TestConcurrentActivationNeverMixesRulesAndPreferences(t *testing.T) {
	makeDoctrine := func(name string) *PreparedDoctrine {
		p, err := PrepareDoctrine(Doctrine{PreferredVehicle: []string{name}}, []*Rule{{Name: name, ConditionSrc: "true", Action: func(env RuleEnv, _ CommandSender) error {
			if len(env.Preferences.Vehicle) != 1 || env.Preferences.Vehicle[0] != name {
				t.Errorf("%s rules saw %v preferences", name, env.Preferences.Vehicle)
			}
			return nil
		}}})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b := makeDoctrine("a"), makeDoctrine("b")
	e, _ := NewEngine(nil)
	e.Activate(a)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			e.Activate(b)
			e.Activate(a)
		}
	}()
	for i := 0; i < 100; i++ {
		_ = e.Evaluate(model.GameState{Tick: i}, "england", nil)
	}
	<-done
}
