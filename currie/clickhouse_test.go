package main

import (
	"bytes"
	"strings"
	"testing"
)

// Every template renders. A mistyped field in a template is a runtime error on
// a page nobody discovers until they are looking at it, and the streamed
// sections are conditional -- so each is rendered both ways here.
func TestTemplatesRender(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	full := view{
		Title:         "game 149 · soviet vs allies · loss",
		DurationTicks: 120000,
		Stream:        sampleStream(),
	}
	cases := []struct {
		name string
		page string
		data any
	}{
		{"report with stream", "report.html.tmpl", full},
		{"report without stream", "report.html.tmpl", view{Title: "game 12", DurationTicks: 1000}},
		{"report, never streamed", "report.html.tmpl", view{
			DurationTicks: 1000,
			Stream:        &StreamView{Note: "this game was not streamed"},
		}},
		{"live", "live.html.tmpl", sampleLive()},
		{"live panel", "livepanel", sampleLive()},
		{"live, nothing shipped", "livepanel", &liveView{Poll: "5s", URL: "/live/panel", Note: "nothing shipped yet"}},
		{"index", "index.html.tmpl", indexView{Dir: "~/.vimy", HasStream: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tmpl.ExecuteTemplate(&buf, tc.page, tc.data); err != nil {
				t.Fatalf("%s: %v", tc.page, err)
			}
			if buf.Len() == 0 {
				t.Fatal("rendered nothing")
			}
		})
	}
}

// The counted sections must never be presentable as the sampled ones. The
// report computes blame from a 1-in-15 export and these rows from an unsampled
// log; a reader who adds them gets a figure that is neither.
func TestReportLabelsTheStreamAsCounted(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "report.html.tmpl", view{DurationTicks: 1000, Stream: sampleStream()}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"counted, not sampled", "one evaluation\n      in fifteen"} {
		if !strings.Contains(out, want) {
			t.Errorf("the page should say %q", want)
		}
	}
}

// The field map is offered only when there are positions to draw. Every game
// played before the units writer has a session and no positions, and a link to
// an empty board reads as the map being broken rather than as the game
// predating it.
func TestFieldLinkOnlyWhenThereArePositions(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	render := func(v *StreamView) string {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "report.html.tmpl", view{DurationTicks: 1000, Stream: v}); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	with := sampleStream()
	if !strings.Contains(render(with), "/field/"+with.Sessions[0].SessionID) {
		t.Error("a session with positions should offer the field")
	}

	without := sampleStream()
	without.Sessions[0].Units = 0
	if strings.Contains(render(without), "/field/") {
		t.Error("a session with no positions must not link to an empty board")
	}
}

// A session that dropped rows makes every count a floor, and the page has to
// say so where the counts are -- not in a log line nobody reads.
func TestDroppedRowsSurfaceAsAFloor(t *testing.T) {
	s := sampleStream()
	s.Sessions[0].RowsDropped = 412
	if !s.Floor() || s.Dropped() != 412 {
		t.Fatalf("floor=%v dropped=%d", s.Floor(), s.Dropped())
	}

	tmpl, _ := parseTemplates()
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "report.html.tmpl", view{DurationTicks: 1000, Stream: s}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "FLOOR") {
		t.Error("a session that dropped rows must say its counts are a floor")
	}
}

// Two standard errors is the bar, and it is deliberately conservative: four
// separate fixes were read as improvements against differences this test calls
// noise.
func TestCohortMovedNeedsMoreThanNoise(t *testing.T) {
	co := &Cohort{
		Rows: []CohortRow{
			{Digest: "old", Rallies: 500, MeanSpread: 19.8, StdErr: 0.20},
			{Digest: "new", Rallies: 500, MeanSpread: 19.2, StdErr: 0.20, Current: true},
		},
	}
	if co.Moved() {
		t.Errorf("0.6 against ±0.8 is inside the noise: %s", co.Delta())
	}
	if !strings.Contains(co.Delta(), "inside the noise") {
		t.Errorf("delta = %q", co.Delta())
	}

	co.Rows[1].MeanSpread = 7.9
	if !co.Moved() {
		t.Errorf("19.8 to 7.9 against ±0.8 is a move: %s", co.Delta())
	}
}

// With nothing before it there is nothing to compare against, and the page has
// to say that rather than report a change from zero.
func TestCohortWithNoPredecessor(t *testing.T) {
	co := &Cohort{Rows: []CohortRow{{Digest: "first", Current: true, MeanSpread: 12}}}
	if co.Previous() != nil || co.Moved() || co.Delta() != "" {
		t.Fatalf("previous=%v moved=%v delta=%q", co.Previous(), co.Moved(), co.Delta())
	}
	if co.Current() == nil {
		t.Fatal("the current digest should still be found")
	}
}

// One rally is a mean. Game 146's mean spread of 17.0 was exactly that, and it
// carried the same visual weight as game 135's 763 rallies.
func TestThinCohort(t *testing.T) {
	if !(CohortRow{Rallies: 1}).Thin() {
		t.Error("a single rally is not a cohort")
	}
	if (CohortRow{Rallies: 763}).Thin() {
		t.Error("763 rallies is")
	}
}

// A rule the stream evaluated and never saw fire is a zero, not a gap in a
// sample. That distinction is the whole reason these rows exist.
func TestSilentRule(t *testing.T) {
	if !(FireRate{Evals: 900, Fired: 0}).Silent() {
		t.Error("evaluated 900 times and never fired is silent")
	}
	f := FireRate{Evals: 900, Fired: 4, FirstFired: 30000, LastFired: 90000}
	if f.Silent() {
		t.Error("four firings is not silent")
	}
	if got := f.FirstPct(120000); got != 0.25 {
		t.Errorf("FirstPct = %v, want 0.25", got)
	}
	if got := f.Span(120000); got != 0.5 {
		t.Errorf("Span = %v, want 0.5", got)
	}
	// A silent rule has no first tick to report, and reporting 0 would read as
	// "acted immediately".
	if got := (FireRate{Fired: 0, FirstFired: 0}).FirstPct(120000); got != 0 {
		t.Errorf("a silent rule has no first act: %v", got)
	}
}

func sampleStream() *StreamView {
	return &StreamView{
		Sessions: []StreamSession{{
			SessionID: "20260921-2210-ab12", StartedAt: 1758000000, GameID: 149,
			RulesDigest: "9f2c1ab77d0e4455", Revision: "a1b2c3d", RowsWritten: 412000,
			Segments: 37, LastIngest: 1758003600, Units: 123614,
		}},
		Silent: []FireRate{
			{Rule: "build-war-factory", Evals: 8123, Fired: 0, Skipped: 12, Rate: 0, RuleSets: 4},
		},
		Firing: []FireRate{
			{Rule: "repair-buildings", Evals: 91234, Fired: 3120, Skipped: 400, Rate: 0.0342, FirstFired: 8000, LastFired: 119000},
		},
		SilentOmitted: 50,
		FiringOmitted: 48,
		Verify: &Verify{
			FirstSwap: 4400, Opening: "seed",
			Rows:    []VerifyRow{{Rule: "produce-vehicle", StreamFired: 412, CounterFired: 409, Drift: 3}},
			PreSwap: []PreSwapRow{{Rule: "deploy-mcv", Fired: 1, First: 10, Last: 10}},
		},
		Cohort: &Cohort{
			Digest: "9f2c1ab77d0e4455",
			Game:   CohortStats{Rallies: 120, MeanSpread: 11.2, PctClumped: 41, MeanMembers: 6.2, Reachable: 0.83},
			Rows: []CohortRow{
				{Digest: "1111aaaa2222bbbb", FirstSeen: 1757000000, Games: 12, Rallies: 900, MeanSpread: 19.8, StdErr: 0.2, PctClumped: 6},
				{Digest: "9f2c1ab77d0e4455", FirstSeen: 1758000000, Games: 3, Rallies: 240, MeanSpread: 11.2, StdErr: 0.4, PctClumped: 41, ModifiedGames: 1, Current: true},
			},
		},
	}
}

func sampleLive() *liveView {
	transit := []TransitPoint{
		{Tick: 0, Samples: 12, Closest: 0.92, Mean: 0.95, Spread: 18, Cohesion: 0.4},
		{Tick: 5000, Samples: 14, Closest: 0.61, Mean: 0.72, Spread: 16, Cohesion: 0.5},
		{Tick: 10000, Samples: 9, Closest: 0.38, Mean: 0.44, Spread: 14, Cohesion: 0.7},
		{Tick: 15000, Samples: 11, Closest: 0.38, Mean: 0.41, Spread: 15, Cohesion: 0.6},
	}
	return &liveView{
		Poll: "5s", URL: "/live/panel", Live: true, Lag: "3s ago", Tick: 18422,
		Session: &StreamSession{
			SessionID: "20260922-1014-ff01", StartedAt: 1758500000, GameID: 0,
			RulesDigest: "9f2c1ab77d0e4455", Revision: "a1b2c3d", Modified: true,
			RowsWritten: 210000, RowsDropped: 0, Segments: 19, LastIngest: 1758503600,
		},
		Rally:   &CohortStats{Rallies: 63, MeanSpread: 12.4, PctClumped: 33, MeanMembers: 5.8, Reachable: 0.77},
		Strikes: []StrikeRow{{Reason: "blind-at-base", Blocked: 41, LastTick: 17900, Share: 1}, {Reason: "en-route", Blocked: 12, LastTick: 16200, Share: 0.29}},
		Transit: transit,
		Spark:   sparkline(transit),
		Rates:   []FireRate{{Rule: "produce-infantry", Evals: 4000, Fired: 210, Rate: 0.0525, FirstFired: 900, LastFired: 18000}},
	}
}

// The trajectory is drawn on a fixed 0..1 axis, never fitted to its own data:
// target_fraction already IS a fraction of the map diagonal, so an unscaled
// line means the same thing in every game. Fitting would make a squad that
// never left the base look like one that crossed the map.
func TestSparklineIsNotRescaled(t *testing.T) {
	near := sparkline([]TransitPoint{{Mean: 0.02, Closest: 0.01}, {Mean: 0.03, Closest: 0.02}})
	far := sparkline([]TransitPoint{{Mean: 0.92, Closest: 0.91}, {Mean: 0.93, Closest: 0.92}})
	if near.Mean == far.Mean {
		t.Fatal("two very different approaches drew the same line")
	}
	// Near the target is near the bottom of the box.
	if !strings.Contains(near.Mean, "117") && !strings.Contains(near.Mean, "116") {
		t.Errorf("a fraction of 0.02 should sit at the bottom of a 120-high box: %q", near.Mean)
	}
}

func TestSparklineStall(t *testing.T) {
	closing := sparkline([]TransitPoint{
		{Closest: 0.9}, {Closest: 0.7}, {Closest: 0.5}, {Closest: 0.2},
	})
	if closing.Stalled {
		t.Error("a squad still closing is not stalled")
	}
	stuck := sparkline([]TransitPoint{
		{Closest: 0.9}, {Closest: 0.4}, {Closest: 0.41}, {Closest: 0.42},
	})
	if !stuck.Stalled {
		t.Error("a closest approach that stops improving is the finding")
	}
	if stuck.Floor != 0.4 {
		t.Errorf("floor = %v, want 0.4", stuck.Floor)
	}
	// Too few points to say anything.
	if s := sparkline([]TransitPoint{{Closest: 0.5}}); s.Mean != "" || s.Stalled {
		t.Error("one point is not a trajectory")
	}
}
