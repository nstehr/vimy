package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/nstehr/vimy/currie/ch"
)

// What the streamed tables can say about a game that the replay cannot.
//
// The replay reads the 1-in-15 export: every row carries the world it was
// judged against, which is what makes the blame analysis possible, and none of
// it can be counted. `build-war-factory` fires once per game in 56 of 69 games
// and appears in the export zero times. The stream is the other half -- one
// row per evaluation, unsampled -- so `fired / evaluated` here means what it
// says.
//
// The two must never be added together, and the page keeps them in separate
// sections for that reason rather than merging them into one ranked list.

// filterForGame is the filter every query below shares.
//
// A game can have more than one session if the sidecar was restarted mid-game,
// and the id arrives only when the retrospective archives -- so this is a
// subquery rather than a constant, and every count is over all of them.
const filterForGame = `session_id IN (
	SELECT session_id FROM stream_sessions FINAL WHERE game_id = {game:UInt32})`

// NEVER ALIAS A RESULT COLUMN TO THE NAME OF A SOURCE COLUMN. ClickHouse
// resolves the alias ahead of the column it shadows, everywhere in the query
// including inside other aggregates. `countIf(fired) AS fired` makes the next
// `countIf(fired)` count a UInt64, and `count() AS fired` in a query that also
// says `WHERE fired` puts an aggregate in WHERE. Both are rejected at run time,
// with a message that does not name the alias, and neither is visible to the
// compiler -- these queries are strings. The integration test in
// clickhouse_live_test.go is what catches them; run it after touching any of
// this.
//
// filterForSession is the live view's filter. A game in progress has no id --
// SQLite hands that out when the retrospective archives -- so the only handle
// on a running game is its session.
const filterForSession = `session_id = {session:String}`

// StreamSession is one run of the sidecar, and what built it.
//
// RowsDropped is the column that decides whether anything below is a number or
// a floor: the log drops rather than stall the game.
type StreamSession struct {
	SessionID   string `json:"session_id"`
	StartedAt   int64  `json:"started_at"`
	GameID      int64  `json:"game_id"`
	RulesDigest string `json:"rules_digest"`
	Revision    string `json:"revision"`
	Modified    bool   `json:"modified"`
	RowsWritten int64  `json:"rows_written"`
	RowsDropped int64  `json:"rows_dropped"`

	// From the shipper's ledger rather than the session: how much has actually
	// landed. A report generated while the tail of a game is still in an open
	// segment is reading an incomplete game, and nothing used to say so.
	Segments   int64 `json:"segments"`
	LastIngest int64 `json:"last_ingest"`

	// Position samples for this session, which is what decides whether the
	// field map is worth offering. Every game played before the units writer
	// has a session and no positions, and a link to an empty board is worse
	// than no link: it reads as the map being broken rather than as the game
	// predating it.
	Units int64 `json:"units"`
}

// HasField reports whether this session has a field worth drawing.
func (s StreamSession) HasField() bool { return s.Units > 0 }

// Started renders the session clock for the template.
func (s StreamSession) Started() time.Time { return time.Unix(s.StartedAt, 0) }

// Floor reports whether every count from this session undercounts.
func (s StreamSession) Floor() bool { return s.RowsDropped > 0 }

// Counted reports whether the session has published its own row total.
//
// It has not, during a game: session.json carries the totals and the sidecar
// writes them when the game ends, so a live session reads zero written and
// zero dropped. Zero dropped is then the ABSENCE of a number, not a clean
// bill, and the page has to say which one it is holding -- claiming a count is
// complete on the strength of a field nobody has filled in yet is exactly the
// kind of confident wrong answer this work exists to stop making.
func (s StreamSession) Counted() bool { return s.RowsWritten > 0 }

// Short is the digest as a reader can hold it in their head.
func (s StreamSession) Short() string {
	if len(s.RulesDigest) > 12 {
		return s.RulesDigest[:12]
	}
	return s.RulesDigest
}

const sessionSQL = `
SELECT
	session_id,
	toUnixTimestamp(started_at) AS started_at,
	game_id,
	rules_digest,
	revision,
	modified,
	toInt64(rows_written)       AS rows_written,
	toInt64(rows_dropped)       AS rows_dropped
FROM stream_sessions FINAL
WHERE game_id = {game:UInt32}
ORDER BY started_at`

// ledger is the shipper's account of one session, joined on afterwards rather
// than in sessionSQL: stream_segments is a different table with a different
// engine, and a session that streamed but has not shipped yet should still
// appear with its zeros showing.
// Grouped rather than one round trip per session: a game has one session
// almost always and two when the sidecar restarted, and either way this is one
// scan of a column the table is ordered by.
const unitsSQL = `
SELECT
	session_id,
	toInt64(count()) AS units
FROM stream_units
WHERE ` + filterForGame + `
GROUP BY session_id`

const ledgerSQL = `
SELECT
	toInt64(count())                AS segments,
	toUnixTimestamp(max(ingested_at)) AS last_ingest
FROM stream_segments FINAL
WHERE session_id = {session:String}`

// streamSessions returns the sessions that played a game, newest last.
//
// An empty slice is the answer for a game played before -stream, or with it
// off. Not an error: most of the archive predates the stream entirely, and a
// report that failed on those would be useless for the thing it was built for.
func streamSessions(ctx context.Context, c *ch.Client, gameID int64) ([]StreamSession, error) {
	sessions, err := ch.Query[StreamSession](ctx, c, sessionSQL, map[string]any{"game": gameID})
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		led, err := ch.One[struct {
			Segments   int64 `json:"segments"`
			LastIngest int64 `json:"last_ingest"`
		}](ctx, c, ledgerSQL, map[string]any{"session": sessions[i].SessionID})
		if err != nil {
			continue // the ledger is for humans; its absence is not a failure
		}
		sessions[i].Segments, sessions[i].LastIngest = led.Segments, led.LastIngest
	}

	// Best-effort: an archive whose ClickHouse predates stream_units has no
	// such table, and that costs the field link rather than the sessions.
	if units, err := ch.Query[struct {
		SessionID string `json:"session_id"`
		Units     int64  `json:"units"`
	}](ctx, c, unitsSQL, map[string]any{"game": gameID}); err == nil {
		for _, u := range units {
			for i := range sessions {
				if sessions[i].SessionID == u.SessionID {
					sessions[i].Units = u.Units
				}
			}
		}
	}
	return sessions, nil
}

// FireRate is one rule's record over a whole game, counted.
//
// The number the report has never had. `rule_firings` in SQLite counts firings
// but has no clause resolution and cannot see anything before the first rule-set
// swap; the export has clause resolution and cannot be counted. This has the
// denominator.
type FireRate struct {
	Rule       string  `json:"rule"`
	RuleSets   int64   `json:"rule_sets"`
	Evals      int64   `json:"evals"`
	Fired      int64   `json:"fires"`
	Skipped    int64   `json:"skips"`
	Rate       float64 `json:"rate"`
	FirstFired int64   `json:"first_fired"`
	LastFired  int64   `json:"last_fired"`

	// Why it never fired, filled in from the replay's blame for silent rules.
	//
	// Without it the section is a flat list and the reader does the ranking. On
	// game 176 that list held produce-scout-vehicle between produce-minelayer
	// and rebuild-iron-curtain: twelve of the fifteen were rebuild rules doing
	// exactly what they should, and one was the most expensive defect in the
	// codebase - a scout gate stuck on from tick 4110 of 54820. It took a
	// separate tool loop and an hour to find what was already on the page.
	//
	// A rule that never fired because its trigger never occurred has no
	// dominant clause. One with a stuck gate is blocked over and over by the
	// same line, and that is the difference worth ranking on.
	Culprit    string
	CulpritAt  string
	CulpritPct float64
	// Won says the replay recorded this rule as the winner of its exclusive
	// category - its condition HELD, and it preempted its siblings - while the
	// stream counted zero firings for it all game. The two halves cannot both
	// be describing the same ticks, and that is the finding: the rule was
	// working when the replay sampled it and stopped at some point after.
	Won bool
}

// silentCulpritFloor is how dominant one clause must be before a silent rule
// counts as gated rather than merely unused. Four fifths: a rule stopped by the
// same line four times in five is not waiting for its moment.
const silentCulpritFloor = 0.8

// Silent reports a rule that was evaluated and never once fired.
//
// Distinct from a rule the export's sampler merely missed, which is the whole
// point of counting: on this table a zero is a zero.
func (f FireRate) Silent() bool { return f.Fired == 0 }

// Span is how much of the game separated the first firing from the last, as a
// share. Acting once is a different thing from acting throughout.
func (f FireRate) Span(duration int) float64 {
	if f.Fired == 0 || duration <= 0 {
		return 0
	}
	return float64(f.LastFired-f.FirstFired) / float64(duration)
}

// FirstPct is how far into the game the rule first acted.
func (f FireRate) FirstPct(duration int) float64 {
	if f.Fired == 0 || duration <= 0 {
		return 0
	}
	return float64(f.FirstFired) / float64(duration)
}

// skipped counts the evaluations an exclusive winner meant were never reached.
// Streamed anyway -- a rule that is always skipped and a rule that is always
// false look identical in a firing count and need opposite fixes.
func fireRatesSQL(filter string) string {
	return fmt.Sprintf(`
SELECT
	rule,
	toInt64(uniq(rule_set))                          AS rule_sets,
	toInt64(count())                                 AS evals,
	toInt64(countIf(fired))                          AS fires,
	toInt64(countIf(skipped))                        AS skips,
	round(ifNotFinite(countIf(fired) / count(), 0), 4) AS rate,
	minIf(tick, fired)                               AS first_fired,
	maxIf(tick, fired)                               AS last_fired
FROM stream_rule_evals
WHERE %s
GROUP BY rule
ORDER BY fires DESC, rule`, filter)
}

func fireRates(ctx context.Context, c *ch.Client, gameID int64) ([]FireRate, error) {
	return ch.Query[FireRate](ctx, c, fireRatesSQL(filterForGame), map[string]any{"game": gameID})
}

// Verify is the stream checked against SQLite's counters.
//
// Two routes to the same measurement -- one counts rows, the other increments
// an integer in the rule loop -- and keeping both means the stream has an
// oracle. This whole line of work started with a confident count off a sampled
// table that was simply wrong, so the check belongs on the page rather than in
// a make target a human remembers to run.
type Verify struct {
	// FirstSwap is the earliest tick running a rule set other than the opening
	// one. Everything before it is invisible to rule_firings by construction:
	// strategist.go flushes the window counters and then attaches them only
	// when a prior record exists, so the seed window is read out and dropped.
	// The comparison is scoped to at-or-after this tick, which is the only
	// like-for-like window.
	FirstSwap int64
	// Opening is the rule set the game started on.
	Opening string
	// Rows is the per-rule comparison, largest disagreement first.
	Rows []VerifyRow
	// PreSwap is what fired before the first swap -- the opening, which the
	// counter structurally cannot see. Not drift, and worth reading alone.
	PreSwap []PreSwapRow
}

// Agrees reports the check passing: exact agreement inside the comparable
// window. The stream is unsampled and has no excuse for a shortfall.
func (v *Verify) Agrees() bool { return v != nil && len(v.Rows) == 0 }

// Drifted counts the rules that disagree.
func (v *Verify) Drifted() int {
	if v == nil {
		return 0
	}
	return len(v.Rows)
}

type VerifyRow struct {
	Rule         string `json:"rule"`
	StreamFired  int64  `json:"stream_fired"`
	CounterFired int64  `json:"counter_fired"`
	Drift        int64  `json:"drift"`
}

type PreSwapRow struct {
	Rule  string `json:"rule"`
	Fired int64  `json:"fires"`
	First int64  `json:"first"`
	Last  int64  `json:"last"`
}

// The opening rule set, and the first tick that is not it.
//
// Two statements rather than one with a WITH alias: the analyzer will not
// resolve a WITH alias referenced from the SELECT list of the same query, and
// the round trip is cheap against a column the table is ordered by.
const openingSQL = `
SELECT argMin(rule_set, tick) AS opening
FROM stream_rule_evals
WHERE ` + filterForGame

const firstSwapSQL = `
SELECT min(tick) AS first_swap
FROM stream_rule_evals
WHERE ` + filterForGame + ` AND rule_set != {opening:String}`

// The counts are cast to Int64 BEFORE subtracting. countIf returns UInt64 and
// a UInt64 subtraction wraps, so a counter ahead of the stream by one would
// report a drift of 18446744073709551615 and read as catastrophe.
const verifySQL = `
SELECT
	rule,
	stream_fired,
	counter_fired,
	stream_fired - counter_fired AS drift
FROM
(
	SELECT rule AS rule, toInt64(countIf(fired)) AS stream_fired
	FROM stream_rule_evals
	WHERE ` + filterForGame + ` AND tick >= {swap:UInt32}
	GROUP BY rule
) AS st
FULL OUTER JOIN
(
	SELECT f.rule_name AS rule, toInt64(sum(f.fire_count)) AS counter_fired
	FROM sqlite('vimy.db', 'rule_firings') AS f
	INNER JOIN sqlite('vimy.db', 'archived_doctrines') AS d ON d.id = f.doctrine_id
	WHERE d.game_id = {game:UInt32}
	GROUP BY rule
) AS ct
USING (rule)
WHERE drift != 0
ORDER BY abs(drift) DESC, rule
LIMIT {limit:UInt32}`

const preSwapSQL = `
SELECT
	rule,
	toInt64(count())   AS fires,
	toInt64(min(tick)) AS first,
	toInt64(max(tick)) AS last
FROM stream_rule_evals
WHERE ` + filterForGame + ` AND fired AND tick < {swap:UInt32}
GROUP BY rule
ORDER BY fires DESC
LIMIT {limit:UInt32}`

func verifyGame(ctx context.Context, c *ch.Client, gameID int64) (*Verify, error) {
	opening, err := ch.One[struct {
		Opening string `json:"opening"`
	}](ctx, c, openingSQL, map[string]any{"game": gameID})
	if err != nil {
		return nil, err
	}
	swap, err := ch.One[struct {
		FirstSwap int64 `json:"first_swap"`
	}](ctx, c, firstSwapSQL, map[string]any{"game": gameID, "opening": opening.Opening})
	if err != nil {
		return nil, err
	}
	// No swap observed means the game never left its opening rule set, and so
	// rule_firings holds nothing at all for it. There is no comparable window
	// to scope to, and comparing anyway would report every rule as drift.
	if swap.FirstSwap == 0 {
		return nil, fmt.Errorf("game %d never swapped rule set; the counters cannot see it", gameID)
	}

	v := &Verify{FirstSwap: swap.FirstSwap, Opening: opening.Opening}
	args := map[string]any{"game": gameID, "swap": swap.FirstSwap, "limit": 40}
	if v.Rows, err = ch.Query[VerifyRow](ctx, c, verifySQL, args); err != nil {
		return nil, err
	}
	if v.PreSwap, err = ch.Query[PreSwapRow](ctx, c, preSwapSQL, args); err != nil {
		return nil, err
	}
	return v, nil
}

// Cohort puts one game against every other game played by the same code.
//
// The question the archive could not answer. 147 games and no column saying
// which rule sources played any of them, so squad spread sat near 20 cells
// against a required 8 through four separate fixes, each judged by eye against
// the next game or two -- against a metric whose game-to-game noise is larger
// than the effect being chased.
type Cohort struct {
	Digest string
	// This game alone.
	Game CohortStats
	// Every digest, oldest first, with the current one marked.
	Rows []CohortRow
}

// Current returns the row for the digest that played this game.
func (c *Cohort) Current() *CohortRow {
	if c == nil {
		return nil
	}
	for i := range c.Rows {
		if c.Rows[i].Current {
			return &c.Rows[i]
		}
	}
	return nil
}

// Previous returns the digest immediately before the current one, which is what
// a before/after actually compares against.
func (c *Cohort) Previous() *CohortRow {
	if c == nil {
		return nil
	}
	for i := range c.Rows {
		if c.Rows[i].Current && i > 0 {
			return &c.Rows[i-1]
		}
	}
	return nil
}

// Moved reports whether the current digest's mean differs from the previous
// one's by more than the two standard errors combined.
//
// Deliberately conservative, and the whole reason the column exists: four
// fixes were read as improvements against differences this test would have
// called noise.
func (c *Cohort) Moved() bool {
	cur, prev := c.Current(), c.Previous()
	if cur == nil || prev == nil {
		return false
	}
	diff := cur.MeanSpread - prev.MeanSpread
	if diff < 0 {
		diff = -diff
	}
	return diff > 2*(cur.StdErr+prev.StdErr)
}

// Delta is the change in mean spread from the previous digest, and how it
// compares to the noise. Empty when there is nothing to compare against.
func (c *Cohort) Delta() string {
	cur, prev := c.Current(), c.Previous()
	if cur == nil || prev == nil {
		return ""
	}
	diff := cur.MeanSpread - prev.MeanSpread
	noise := 2 * (cur.StdErr + prev.StdErr)
	verdict := "inside the noise"
	if c.Moved() {
		verdict = "outside the noise"
	}
	return fmt.Sprintf("%+.2f cells against ±%.2f — %s", diff, noise, verdict)
}

type CohortStats struct {
	Rallies     int64   `json:"rallies"`
	MeanSpread  float64 `json:"mean_spread"`
	PctClumped  float64 `json:"pct_clumped"`
	MeanMembers float64 `json:"mean_members"`
	// Reachable is commandable over members: the share of the squad the rally
	// order actually went to. Near 1 with a wide spread means the radius is the
	// fault; well below means the rally cannot reach the squad, which is a
	// different bug with a different fix.
	//
	// It cannot exceed 1 -- commandable is a subset of members -- so a value
	// above 1 is not a good reading, it is a broken one. A joiner dispatched
	// twice was enlisted twice, and the duplicate counted in the numerator and
	// not the denominator. Impossible marks that rather than letting 1.13 read
	// as the healthiest possible result.
	Reachable float64 `json:"reachable"`
	// Rallies whose commandable count exceeded their membership, which cannot
	// happen and means the roster held duplicates.
	Impossible int64 `json:"impossible"`
}

// Broken reports a rally sample that cannot be right.
//
// The commandable count is a subset of the membership, so the ratio cannot
// exceed 1. When it does, the roster held the same unit twice and the duplicate
// counted in the numerator only -- which read as 1.13, the healthiest possible
// result, on the one panel that would have shown the bug.
func (c CohortStats) Broken() bool { return c.Impossible > 0 }

type CohortRow struct {
	Digest     string  `json:"digest"`
	FirstSeen  int64   `json:"first_seen"`
	Games      int64   `json:"games"`
	Rallies    int64   `json:"rallies"`
	MeanSpread float64 `json:"mean_spread"`
	PctClumped float64 `json:"pct_clumped"`
	StdErr     float64 `json:"stderr"`
	// Games built from a dirty tree. A before/after that spans one of these is
	// not an answer, so it is shown rather than quietly averaged in.
	ModifiedGames int64 `json:"modified_games"`

	Current bool `json:"-"`
}

// Seen renders the first game under this digest.
func (r CohortRow) Seen() time.Time { return time.Unix(r.FirstSeen, 0) }

// Short is the digest, shortened.
func (r CohortRow) Short() string {
	if len(r.Digest) > 12 {
		return r.Digest[:12]
	}
	return r.Digest
}

// Thin marks a cohort too small to say anything with. One rally is a mean, and
// game 146's mean spread of 17.0 was exactly that.
func (r CohortRow) Thin() bool { return r.Rallies < 30 }

// Pooled at the rally, not averaged over per-game means: game 146's 17.0 was a
// single rally and a per-game mean gives it the same weight as game 135's 763.
const cohortSQL = `
SELECT
	s.rules_digest                                   AS digest,
	toUnixTimestamp(min(s.started_at))               AS first_seen,
	toInt64(uniq(s.session_id))                      AS games,
	toInt64(count())                                 AS rallies,
	round(avg(e.spread), 2)                          AS mean_spread,
	round(100 * countIf(e.spread <= 8) / count(), 1) AS pct_clumped,
	round(ifNotFinite(stddevSamp(e.spread) / sqrt(count()), 0), 3) AS stderr,
	toInt64(uniqIf(s.session_id, s.modified))        AS modified_games
FROM stream_events AS e
INNER JOIN stream_sessions AS s FINAL USING (session_id)
WHERE e.kind = 'rally'
GROUP BY digest
ORDER BY first_seen`

func ralliesSQL(filter string) string {
	return fmt.Sprintf(`
SELECT
	toInt64(count())                                          AS rallies,
	round(ifNotFinite(avg(spread), 0), 2)                     AS mean_spread,
	round(ifNotFinite(100 * countIf(spread <= 8) / count(), 0), 1) AS pct_clumped,
	round(ifNotFinite(avg(members), 0), 2)                    AS mean_members,
	round(ifNotFinite(avg(idle) / avg(members), 0), 2)        AS reachable,
	toInt64(countIf(idle > members))                          AS impossible
FROM stream_events
WHERE kind = 'rally' AND %s`, filter)
}

func cohortFor(ctx context.Context, c *ch.Client, gameID int64, digest string) (*Cohort, error) {
	rows, err := ch.Query[CohortRow](ctx, c, cohortSQL, nil)
	if err != nil {
		return nil, err
	}
	mine, err := ch.One[CohortStats](ctx, c, ralliesSQL(filterForGame), map[string]any{"game": gameID})
	if err != nil && !errors.Is(err, ch.ErrNoRows) {
		return nil, err
	}
	co := &Cohort{Digest: digest, Game: mine, Rows: rows}
	for i := range co.Rows {
		co.Rows[i].Current = co.Rows[i].Digest == digest
	}
	return co, nil
}

// StreamView is every streamed section of one game's report.
//
// Each piece fails on its own. A game with no session, a ClickHouse that is
// down, a `04_stream.sql` that was never loaded -- all of them cost the page
// these sections and nothing else. The blame analysis is what the reader came
// for and it does not depend on any of this.
type StreamView struct {
	Sessions []StreamSession
	// Two lists, not one ranked table. A rule that never fired and a rule that
	// fired constantly are both answers to "what did the game do", but they do
	// not rank against each other: ordering them together buried every firing
	// rule beneath 51 silent ones and left a section called "what actually
	// fired" showing nothing that fired.
	//
	// Silent is ordered by evaluations, because that is the interesting axis
	// for a rule that never fired -- 3,567 chances refused is a finding and
	// 139 is a rule that barely came up.
	Silent []FireRate
	Firing []FireRate
	// What each list had no room for. Counted, because a silent cap reads as
	// "this is all of them".
	SilentOmitted int
	FiringOmitted int
	Verify        *Verify
	VerifyNote    string
	Cohort        *Cohort
	// Why a section is missing, when it is missing for a reason worth saying.
	Note string
}

// Streamed reports whether there is anything to show at all.
func (s *StreamView) Streamed() bool { return s != nil && len(s.Sessions) > 0 }

// Floor reports whether any session dropped rows, which makes every count on
// the page a floor rather than a number.
func (s *StreamView) Floor() bool {
	if s == nil {
		return false
	}
	for _, sess := range s.Sessions {
		if sess.Floor() {
			return true
		}
	}
	return false
}

// Dropped totals the rows lost to a full buffer.
func (s *StreamView) Dropped() int64 {
	var n int64
	if s == nil {
		return 0
	}
	for _, sess := range s.Sessions {
		n += sess.RowsDropped
	}
	return n
}

// cap_ truncates a list and reports what it dropped.
func cap_(rates []FireRate, n int) ([]FireRate, int) {
	if len(rates) <= n {
		return rates, 0
	}
	return rates[:n], len(rates) - n
}

// loadStream assembles the streamed half of a report.
//
// Best-effort throughout, and per section: a failure in the cohort query is
// not a reason to withhold the fire rates. Every absence is a skipped section
// rather than an error, which is the same rule addOutcome follows and for the
// same reason -- "did not fire" and "was not counting" are different findings.
func loadStream(ctx context.Context, c *ch.Client, gameID int64, topRates int) *StreamView {
	if c == nil {
		return nil
	}
	v := &StreamView{}

	sessions, err := streamSessions(ctx, c, gameID)
	if err != nil {
		slog.Warn("stream sessions", "game", gameID, "error", err)
		v.Note = "ClickHouse could not be reached: " + err.Error()
		return v
	}
	if len(sessions) == 0 {
		v.Note = "this game was not streamed — played before `-stream`, or with it off"
		return v
	}
	v.Sessions = sessions

	if rates, err := fireRates(ctx, c, gameID); err != nil {
		slog.Warn("fire rates", "game", gameID, "error", err)
	} else {
		for _, r := range rates {
			if r.Silent() {
				v.Silent = append(v.Silent, r)
			} else {
				v.Firing = append(v.Firing, r)
			}
		}
		// Most chances refused first, then the ones that at least entered an
		// exclusive contest. Rules evaluated every cycle all tie on the first
		// key, so without the second the list degrades to alphabetical and the
		// order stops carrying meaning.
		sort.SliceStable(v.Silent, func(i, j int) bool {
			a, b := v.Silent[i], v.Silent[j]
			// A stuck gate first, whatever the evaluation counts. Rules
			// evaluated every cycle all tie on evals, so ordering by that alone
			// left the one rule with a broken condition indistinguishable from
			// a dozen rebuild rules waiting for something to be destroyed.
			if (a.CulpritPct >= silentCulpritFloor) != (b.CulpritPct >= silentCulpritFloor) {
				return a.CulpritPct >= silentCulpritFloor
			}
			if a.CulpritPct >= silentCulpritFloor && a.CulpritPct != b.CulpritPct {
				return a.CulpritPct > b.CulpritPct
			}
			if a.Evals != b.Evals {
				return a.Evals > b.Evals
			}
			if a.Skipped != b.Skipped {
				return a.Skipped > b.Skipped
			}
			return a.Rule < b.Rule
		})
		v.Silent, v.SilentOmitted = cap_(v.Silent, topRates)
		v.Firing, v.FiringOmitted = cap_(v.Firing, topRates)
	}

	if ver, err := verifyGame(ctx, c, gameID); err != nil {
		slog.Warn("verify", "game", gameID, "error", err)
		v.VerifyNote = err.Error()
	} else {
		v.Verify = ver
	}

	if co, err := cohortFor(ctx, c, gameID, sessions[len(sessions)-1].RulesDigest); err != nil {
		slog.Warn("cohort", "game", gameID, "error", err)
	} else {
		v.Cohort = co
	}
	return v
}

// explainSilent fills in why each silent rule never fired.
//
// The stream counts firings exactly and so can say a zero is a zero; the replay
// samples states but records which clause stopped each rule. Neither half is
// the finding on its own: "produce-scout-vehicle was evaluated 5368 times and
// never fired" is a curiosity until you see it was stopped every time by the
// same line, and then it is a stuck gate.
func explainSilent(v *StreamView, dead []deadRule, never []preemptedRule) {
	if v == nil || len(v.Silent) == 0 {
		return
	}
	// Rules the replay saw winning their category. A winner that never fired is
	// a contradiction between the two halves and outranks a plain stuck gate:
	// produce-scout-vehicle preempted five siblings in game 176 and fired zero
	// times, which no single view of the game says on its own.
	won := make(map[string]bool)
	for _, p := range never {
		for _, w := range p.LostTo {
			won[w] = true
		}
	}
	for i := range v.Silent {
		v.Silent[i].Won = won[v.Silent[i].Rule]
	}
	by := make(map[string]deadRule, len(dead))
	for _, d := range dead {
		by[d.Name] = d
	}
	for i := range v.Silent {
		d, ok := by[v.Silent[i].Rule]
		if !ok || d.Culprit == nil || d.Blocked == 0 {
			continue
		}
		v.Silent[i].Culprit = d.Culprit.Source
		v.Silent[i].CulpritAt = fmt.Sprintf("%s:%d", d.Culprit.File, d.Culprit.Line)
		v.Silent[i].CulpritPct = float64(d.Culprit.Blocked) / float64(d.Blocked)
	}
	// Re-sort: the ranking depends on what was just filled in.
	sort.SliceStable(v.Silent, func(i, j int) bool {
		a, b := v.Silent[i], v.Silent[j]
		if a.Won != b.Won {
			return a.Won
		}
		if (a.CulpritPct >= silentCulpritFloor) != (b.CulpritPct >= silentCulpritFloor) {
			return a.CulpritPct >= silentCulpritFloor
		}
		if a.CulpritPct >= silentCulpritFloor && a.CulpritPct != b.CulpritPct {
			return a.CulpritPct > b.CulpritPct
		}
		return a.Evals > b.Evals
	})
}

// Gated reports a silent rule stopped overwhelmingly by one clause: a stuck
// gate rather than a rule waiting for its trigger.
func (f FireRate) Gated() bool { return f.CulpritPct >= silentCulpritFloor }
