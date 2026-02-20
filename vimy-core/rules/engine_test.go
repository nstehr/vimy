package rules

import "testing"

func TestDefaultRulesCompile(t *testing.T) {
	engine, err := NewEngine(DefaultRules())
	if err != nil {
		t.Fatalf("NewEngine(DefaultRules()) failed: %v", err)
	}
	if len(engine.rules) != 11 {
		t.Errorf("expected 11 rules, got %d", len(engine.rules))
	}
	// Verify priority ordering (descending).
	for i := 1; i < len(engine.rules); i++ {
		if engine.rules[i].Priority > engine.rules[i-1].Priority {
			t.Errorf("rules not sorted by priority: %s (%d) > %s (%d)",
				engine.rules[i].Name, engine.rules[i].Priority,
				engine.rules[i-1].Name, engine.rules[i-1].Priority)
		}
	}
}
