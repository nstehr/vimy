package main

import (
	"context"
	"os"
	"testing"

	"github.com/nstehr/vimy/currie/ch"
)

// Against a real server.
//
//	CURRIE_CH=http://localhost:8123 go test ./ -run Live
//
// Skipped by default, because it needs `make -C clickhouse up` and a game that
// has actually been streamed. Worth keeping and worth running after any edit
// to the SQL above: every query in this file is a string, so the compiler
// checks none of it and a typo is a section that silently disappears from the
// page. That is the failure mode this whole line of work exists to remove.
func liveClient(t *testing.T) *ch.Client {
	t.Helper()
	url := os.Getenv("CURRIE_CH")
	if url == "" {
		t.Skip("set CURRIE_CH to run against a real ClickHouse")
	}
	c := ch.New(url, "currie", "currie", "currie")
	if err := c.Ping(context.Background()); err != nil {
		t.Skipf("no ClickHouse at %s: %v", url, err)
	}
	return c
}

// anyStreamedGame returns a game id the stream has, or skips.
func anyStreamedGame(t *testing.T, c *ch.Client) int64 {
	t.Helper()
	// The alias must not repeat the column name. ClickHouse resolves an alias
	// ahead of the column it shadows, so `max(game_id) AS game_id` turns
	// `WHERE game_id > 0` into an aggregate in WHERE and the query is rejected.
	row, err := ch.One[struct {
		Latest int64 `json:"latest"`
	}](context.Background(), c,
		`SELECT toInt64(max(game_id)) AS latest FROM stream_sessions FINAL WHERE game_id > 0`, nil)
	if err != nil {
		t.Fatalf("finding a streamed game: %v", err)
	}
	if row.Latest == 0 {
		t.Skip("no archived game has been streamed")
	}
	return row.Latest
}

func TestLiveQueriesParse(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()
	game := anyStreamedGame(t, c)

	t.Run("sessions", func(t *testing.T) {
		got, err := streamSessions(ctx, c, game)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("no session for a game chosen because it has one")
		}
		if got[0].RulesDigest == "" {
			t.Error("a session with no digest cannot be compared against anything")
		}
	})

	t.Run("fire rates", func(t *testing.T) {
		rates, err := fireRates(ctx, c, game)
		if err != nil {
			t.Fatal(err)
		}
		if len(rates) == 0 {
			t.Fatal("a streamed game evaluated no rules")
		}
		for _, r := range rates {
			if r.Fired > r.Evals {
				t.Fatalf("%s fired %d times in %d evaluations", r.Rule, r.Fired, r.Evals)
			}
		}
	})

	// The one with a sign. countIf returns UInt64 and an unguarded UInt64
	// subtraction wraps, so a counter ahead of the stream by one reports
	// 18446744073709551615 and reads as catastrophe.
	t.Run("verify drift is signed", func(t *testing.T) {
		v, err := verifyGame(ctx, c, game)
		if err != nil {
			t.Skipf("not comparable: %v", err)
		}
		for _, row := range v.Rows {
			if row.Drift != row.StreamFired-row.CounterFired {
				t.Fatalf("%s: drift %d does not equal %d - %d",
					row.Rule, row.Drift, row.StreamFired, row.CounterFired)
			}
			if row.Drift > 1<<40 || row.Drift < -(1<<40) {
				t.Fatalf("%s: drift %d — an unsigned subtraction wrapped", row.Rule, row.Drift)
			}
		}
	})

	t.Run("cohort", func(t *testing.T) {
		sessions, err := streamSessions(ctx, c, game)
		if err != nil || len(sessions) == 0 {
			t.Skip("no session")
		}
		co, err := cohortFor(ctx, c, game, sessions[0].RulesDigest)
		if err != nil {
			t.Fatal(err)
		}
		if co.Current() == nil {
			t.Fatal("this game's own digest is missing from the cohort table")
		}
		for _, r := range co.Rows {
			if r.PctClumped < 0 || r.PctClumped > 100 {
				t.Fatalf("%s: %.1f%% clumped", r.Short(), r.PctClumped)
			}
		}
	})

	t.Run("live panel", func(t *testing.T) {
		v := loadLive(ctx, c, 10)
		if !v.Has() {
			t.Fatalf("no session found: %s", v.Note)
		}
		for _, p := range v.Transit {
			if p.Mean < 0 || p.Mean > 1.5 {
				t.Fatalf("target_fraction %v is not a fraction of the diagonal", p.Mean)
			}
		}
	})
}
