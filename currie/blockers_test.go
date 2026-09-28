package main

import (
	"bytes"
	"strings"
	"testing"
)

// The four line-keyed views join on file:line, and a row carries whichever of
// them had something to say about it.
func TestBlockersJoinOnTheLine(t *testing.T) {
	v := &view{
		Sites: []site{
			{File: "core.vy", Line: 12, Source: "cash >= cost", Sole: 1615, Rules: []string{"a", "b"}},
			{File: "micro.vy", Line: 94, Source: "require count(idle-scouts) > 0", Sole: 1233, Rules: []string{"c"}},
		},
		Gates: []Gate{
			{File: "../vimy-core/rules/vy/core.vy", Line: 12, Source: "cash >= cost", Relief: 3861},
			// A line no site ranked: still worth a row.
			{File: "buildings.vy", Line: 7, Source: "cash >= 2000", Relief: 0},
		},
		Sensitivity: []Sensitivity{{File: "micro.vy", Line: 94, Source: "require count(idle-scouts) > 0", Doctrinal: true, Knob: "aggression"}},
		Chains:      []Chain{{Rule: "lay-mines", File: "micro.vy", Line: 94, Clause: "require count(idle-scouts) > 0", Thing: "scout", Maker: "produce-scout-vehicle"}},
	}
	got := blockers(v)
	if len(got) != 3 {
		t.Fatalf("want two ranked lines and one gate-only line, got %d", len(got))
	}
	if !got[0].Ranked || got[0].Gate == nil || got[0].Gate.Relief != 3861 {
		t.Errorf("the gate did not join onto core.vy:12 through its full path: %+v", got[0])
	}
	if got[1].Sens == nil || len(got[1].Chains) != 1 {
		t.Errorf("sensitivity and chain did not join onto micro.vy:94: %+v", got[1])
	}
	if got[2].Ranked || got[2].Line != 7 {
		t.Errorf("the gate-only line should be present and unranked: %+v", got[2])
	}
	if !got[0].Tunable() || got[2].Tunable() {
		t.Errorf("Tunable read the wrong way round: %v %v", got[0].Tunable(), got[2].Tunable())
	}
}

// Everything the merged section can show renders, including the diagnostics
// that moved inside the stream's fold and so are addressed through $.
func TestReportRendersTheMergedBlockers(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	v := view{
		Title: "game 176", DurationTicks: 1000, States: 1233, Windows: 40,
		Stream: sampleStream(),
		Blockers: []Blocker{{
			site:   site{File: "micro.vy", Line: 94, Source: "require count(idle-scouts) > 0", Sole: 1233, Rules: []string{"scout-with-vehicle"}},
			Ranked: true,
			Gate:   &Gate{File: "micro.vy", Line: 94, Source: "cash >= cost", Relief: 12, Blocked: 40, AtLow: 100, AtHigh: 2000, ReliefLow: 1, ReliefHigh: 50},
			Sens:   &Sensitivity{Doctrinal: true, Knob: "aggression", RateLow: 0.27, RateHigh: 0.82, Windows: 40},
			Chains: []Chain{{Rule: "scout-with-vehicle", Thing: "scout", Maker: "produce-scout-vehicle", MakerCulprit: "not has-enemy-intel()", MakerFile: "production.vy", MakerLine: 61}},
		}},
		FiredAnyway: []deadRule{{
			Name: "harass-harvesters", Category: "micro", Seen: 900, Matched: 7, Acted: 7,
			Culprit: &clauseReport{Source: "require enemy-harvesters() > 0"},
		}},
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "report.html.tmpl", v); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"no <b>scout</b> existed",
		"produce-scout-vehicle",
		"a dial:",
		"blocks when aggression is high",
		"<b>aggression</b>",
		// Moved into the stream's fold, where the view is no longer the dot.
		"harass-harvesters",
		"Can these numbers be trusted?",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the merged blocker section dropped %q", want)
		}
	}
	// The five separate sections are gone; one heading answers for all of them.
	for _, gone := range []string{"Missing things, and what should have made them", "Gates worth moving", "Block rate across doctrine windows"} {
		if strings.Contains(out, gone) {
			t.Errorf("%q is still its own section", gone)
		}
	}
}
