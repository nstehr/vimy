package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/nstehr/vimy/currie/baml_client"
	"github.com/nstehr/vimy/currie/baml_client/types"
	"github.com/nstehr/vimy/currie/ch"
)

// The post mortem as an investigation.
//
// ReadPostMortem hands the model one bundle of facts chosen in advance, so the
// bundle has to anticipate the question. Every real diagnosis this project has
// produced came from following a thread instead — blockers, then the squad
// timeline, then the field at the tick where it collapsed — and no fixed
// projection is all three at once.
//
// The constraint that shaped the original still holds: the model must not be
// able to make a claim the reader cannot check. It is met differently here.
// every call and every result is recorded and rendered beside the findings, so
// the reader sees the evidence chain rather than conclusions drawn from a
// projection they have to take on trust.

// maxInvestigationSteps bounds one game's investigation. An analyst still
// asking on the ninth call has lost the thread, and each call is a model round
// trip against a page a human is waiting on.
const maxInvestigationSteps = 8

// maxMalformedSteps is how many unparseable replies one investigation absorbs
// before giving up. Enough for a slip, few enough that a model which cannot
// produce the schema at all does not spend the whole budget discovering that.
const maxMalformedSteps = 2

// TraceStep is one call and what it returned, for the page.
type TraceStep struct {
	Tool   string
	Reason string
	Result string
}

// investigator runs the loop. Nil ch means no telemetry, in which case the
// tools say so rather than inventing an answer.
type investigator struct {
	ch *ch.Client
}

func (iv *investigator) Investigate(ctx context.Context, r *Replay) (*Insight, []TraceStep, bool, error) {
	f := facts(r)
	session := iv.sessionFor(ctx, r.Game.ID)
	// Decided before the loop runs, not after: by the time eight calls have
	// finished, more of the stream may have landed and the reading would then
	// be cached as though it had seen it.
	settled := iv.settled(ctx, session)

	var history []types.AgentMessage
	var trace []TraceStep

	// A malformed step costs a turn, not the investigation.
	//
	// Game 176 died at step 5: the model tried to finish and omitted two fields
	// it had nothing to say for, so BAML rejected all five candidate parses and
	// the whole run was lost along with four good tool results. Optional fields
	// make that rarer; this makes it survivable. The failure is fed back as a
	// tool message, which is the only thing that has a chance of correcting it.
	malformed := 0

	for step := range maxInvestigationSteps {
		res, err := baml_client.InvestigateGameStep(ctx, f, toolMenu, history)
		if err != nil {
			malformed++
			if malformed > maxMalformedSteps {
				return nil, trace, settled, fmt.Errorf("investigate step %d: %w", step, err)
			}
			slog.Warn("malformed investigation step", "game", r.Game.ID, "step", step, "error", err)
			history = append(history,
				types.AgentMessage{Role: "user", Content: "That response could not be parsed. Emit exactly one tool object, with its `tool` field set. Every other field is optional - omit what you have nothing to say for rather than leaving the object incomplete."})
			continue
		}

		if final := res.AsFinalInsightTool(); final != nil {
			ins := &Insight{Summary: final.Summary, Suggestion: deref(final.Suggestion), Caveat: deref(final.Caveat)}
			if a := final.Directive_advice; a != nil {
				ins.Directive = &DirectiveAdvice{Diagnosis: a.Diagnosis, Revision: a.Revision, Confidence: a.Confidence}
			}
			for _, fd := range final.Findings {
				ins.Findings = append(ins.Findings, Finding{Claim: fd.Claim, Evidence: fd.Evidence, Confidence: fd.Confidence})
			}
			return ins, trace, settled, nil
		}

		name, reason, result := iv.run(ctx, session, res)
		if name == "" {
			// A tool we do not serve. Say so in the transcript rather than
			// looping on it silently.
			name, result = "unknown", "no such tool"
		}
		slog.Debug("investigation step", "game", r.Game.ID, "tool", name, "step", step)
		trace = append(trace, TraceStep{Tool: name, Reason: reason, Result: result})
		history = append(history,
			types.AgentMessage{Role: "assistant", Content: formatCall(name, reason)},
			types.AgentMessage{Role: "user", Content: result})
	}

	// Out of budget without a conclusion. Returning the trace anyway: eight
	// tool results are worth reading even when nothing was concluded from them.
	return nil, trace, settled, fmt.Errorf("no conclusion after %d steps", maxInvestigationSteps)
}

// deref flattens an optional string. Every field but the discriminator is
// optional now, because a step that fails to parse costs the whole
// investigation rather than one turn.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func formatCall(name, reason string) string {
	b, err := json.Marshal(map[string]string{"tool": name, "reason": reason})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// sessionFor links the archived game to its telemetry. Empty when the game
// predates streaming, which the tools report rather than failing on.
func (iv *investigator) sessionFor(ctx context.Context, gameID int64) string {
	if iv.ch == nil {
		return ""
	}
	type row struct {
		ID string `json:"session_id"`
	}
	s, err := ch.One[row](ctx, iv.ch,
		`SELECT session_id FROM stream_sessions WHERE game_id = {game:Int64} ORDER BY started_at DESC LIMIT 1`,
		map[string]any{"game": gameID})
	if err != nil {
		return ""
	}
	return s.ID
}

// toolMenu is what the model is told it can ask for. Written as guidance rather
// than a schema dump: the schema is in the output format already, and what the
// model needs is when each is worth spending a step on.
const toolMenu = `
squad_timeline — how the attack squad behaved over the game: size, how much of
  it was together, how strung out, and how close it got to its target. This is
  where "the squad arrived" is separated from "two stragglers arrived", and
  where an approach that oscillates instead of closing shows up.

strike_blockers — why strikes did not happen, counted by reason. unclumped means
  the squad failed its own cohesion gate; no-target-en-route means it was
  walking with nothing to aim at; blind-at-base means it ARRIVED and could see
  nothing, which is a targeting or intel failure rather than a movement one.
  Counts are not causes: read them against the timeline.

field_at — every actor at one tick, with the threat map: ours, enemies seen,
  what the AI merely believes is there, capturables, and the danger field the
  approach router scores corridors against. An empty threat field means nothing
  was scouted, NOT that the ground was safe.

query — one SELECT against the telemetry, for anything the tools above do not
  answer. The connection is read-only and results are capped, so a bad query
  costs a turn and nothing else. Pass the session id as {session:String}.

  stream_units(session_id, tick, unit_id, type, side, x, y, hp, idle,
    is_building, remembered) - every actor at a sampled tick, 20 ticks apart.
    side is ours|enemy|neutral. remembered means the AI believes it is there
    rather than currently seeing it.
  stream_events(session_id, tick, kind, squad, reason, members, idle, near,
    spread, attrs) - kind is rally|transit|strike-blocked|approach. members and
    spread mean the same for every kind; idle and near do NOT - idle is the
    subset an order can reach (rally), near is the subset inside the cohesion
    radius (transit). attrs['target_fraction'] is distance to target over the
    map diagonal.
  stream_threat(session_id, tick, col, row, value) - the danger map, sparse,
    on a 32x32 grid, sampled every 200 ticks.
  stream_rule_evals(session_id, tick, rule, rule_set, state, fired, skipped) -
    every rule evaluation, unsampled. Ten million rows; always filter by
    session and rule.
  stream_sessions(session_id, started_at, game_id, revision, modified,
    terrain...) - revision is the sidecar build, which says whether a fix was
    in this game.
  games(id, duration_ticks, won, engine_buildings_killed, engine_earned,
    engine_kills_cost, engine_deaths_cost, our_army_peak, infantry_lost...)

compare_games — the same headline numbers across recent games, to tell what is
  particular to this one from what is true of the run. Income per tick has
  predicted the outcome in every game measured.
`

// run executes whichever tool the model picked. Returns the tool name, the
// model's stated reason, and the result to feed back.
func (iv *investigator) run(ctx context.Context, session string, res types.Union6CompareGamesToolOrFieldAtToolOrFinalInsightToolOrQueryToolOrSquadTimelineToolOrStrikeBlockersTool) (string, string, string) {
	switch {
	case res.AsSquadTimelineTool() != nil:
		return "squad_timeline", deref(res.AsSquadTimelineTool().Reason), iv.squadTimeline(ctx, session)
	case res.AsStrikeBlockersTool() != nil:
		return "strike_blockers", deref(res.AsStrikeBlockersTool().Reason), iv.strikeBlockers(ctx, session)
	case res.AsFieldAtTool() != nil:
		t := res.AsFieldAtTool()
		// No tick given means "wherever the game ended", which is a reasonable
		// default and better than refusing the call.
		tick := 0
		if t.Tick != nil {
			tick = int(*t.Tick)
		}
		return "field_at", deref(t.Reason), iv.fieldAt(ctx, session, tick)
	case res.AsQueryTool() != nil:
		t := res.AsQueryTool()
		return "query", deref(t.Reason), iv.rawQuery(ctx, session, t.Sql)
	case res.AsCompareGamesTool() != nil:
		return "compare_games", deref(res.AsCompareGamesTool().Reason), iv.compareGames(ctx)
	}
	return "", "", ""
}

// fieldAtAbsentFeedNote is what a missing feed says, kept as a constant so the
// distinction between "not recorded" and "recorded and empty" is testable.
const fieldAtAbsentFeedNote = "threat map: NOT RECORDED for this game. The feed did not exist when it was played, so nothing can be concluded about what had or had not been scouted. Do not read this as an empty map.\n"

// noTelemetry is returned rather than an empty result so the model knows the
// difference between "nothing happened" and "this game predates the feed".
const noTelemetry = "no telemetry for this game: it was played before the stream existed, or the shipper was not running. Do not read this as an absence of activity."

func (iv *investigator) squadTimeline(ctx context.Context, session string) string {
	if iv.ch == nil || session == "" {
		return noTelemetry
	}
	type row struct {
		Band     int     `json:"band"`
		Samples  int     `json:"samples"`
		Members  float64 `json:"members"`
		Together float64 `json:"together"`
		Spread   float64 `json:"spread"`
		Closest  float64 `json:"closest"`
	}
	rows, err := ch.Query[row](ctx, iv.ch,
		`SELECT intDiv(tick, 10000)*10000 AS band, count() AS samples,
		        round(avg(members),1) AS members, round(avg(near),1) AS together,
		        round(avg(spread),1) AS spread,
		        round(min(attrs['target_fraction']),3) AS closest
		 FROM stream_events
		 WHERE session_id = {session:String} AND kind = 'transit'
		 GROUP BY band ORDER BY band`,
		map[string]any{"session": session})
	if err != nil || len(rows) == 0 {
		return "no transit samples: the squad never moved on a target."
	}
	var b strings.Builder
	b.WriteString("squad on the way in, by tick band. closest is distance to target as a fraction of the map diagonal, so lower is nearer; together is how many members were inside the cohesion radius.\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "tick %d: %d samples, %.1f members, %.1f together, spread %.1f, closest %.3f\n",
			r.Band, r.Samples, r.Members, r.Together, r.Spread, r.Closest)
	}
	return b.String()
}

func (iv *investigator) strikeBlockers(ctx context.Context, session string) string {
	if iv.ch == nil || session == "" {
		return noTelemetry
	}
	type row struct {
		Reason string `json:"reason"`
		N      int    `json:"n"`
		Last   int    `json:"last"`
	}
	rows, err := ch.Query[row](ctx, iv.ch,
		`SELECT reason, count() AS n, max(tick) AS last FROM stream_events
		 WHERE session_id = {session:String} AND kind IN ('strike-blocked','approach')
		 GROUP BY reason ORDER BY n DESC`,
		map[string]any{"session": session})
	if err != nil || len(rows) == 0 {
		return "no blocked strikes recorded."
	}
	var b strings.Builder
	b.WriteString("why strikes did not happen, and how the approach router decided. already-open on the approach means the direct corridor scored clear, which with little intel means unscouted rather than safe.\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%s: %d (last at tick %d)\n", r.Reason, r.N, r.Last)
	}
	return b.String()
}

func (iv *investigator) fieldAt(ctx context.Context, session string, tick int) string {
	if iv.ch == nil || session == "" {
		return noTelemetry
	}
	type row struct {
		Side       string `json:"side"`
		Building   bool   `json:"is_building"`
		Remembered bool   `json:"remembered"`
		N          int    `json:"n"`
	}
	rows, err := ch.Query[row](ctx, iv.ch,
		`SELECT side, is_building, remembered, count() AS n FROM stream_units
		 WHERE session_id = {session:String}
		   AND tick = (SELECT max(tick) FROM stream_units
		               WHERE session_id = {session:String} AND tick <= {tick:UInt32})
		 GROUP BY side, is_building, remembered ORDER BY n DESC`,
		map[string]any{"session": session, "tick": tick})
	if err != nil || len(rows) == 0 {
		return fmt.Sprintf("no field sample at or before tick %d.", tick)
	}
	// Whether the feed ran at all, before reading anything into a zero.
	//
	// The first live investigation concluded that game 174 failed for want of
	// scouting, on the strength of this tool reporting an empty threat map.
	// The threat emitter did not exist when that game was played. Zero rows
	// meant no feed, and the tool asserted it meant no intel - absence of data
	// presented as evidence, which is the exact error the tool menu warns the
	// model about.
	type trow struct {
		Zones int     `json:"zones"`
		Peak  float64 `json:"peak"`
	}
	type anyrow struct {
		N int `json:"n"`
	}
	ever, _ := ch.One[anyrow](ctx, iv.ch,
		`SELECT count() AS n FROM stream_threat WHERE session_id = {session:String}`,
		map[string]any{"session": session})
	th, _ := ch.One[trow](ctx, iv.ch,
		`SELECT count() AS zones, round(max(value),2) AS peak FROM stream_threat
		 WHERE session_id = {session:String}
		   AND tick = (SELECT max(tick) FROM stream_threat
		               WHERE session_id = {session:String} AND tick <= {tick:UInt32})`,
		map[string]any{"session": session, "tick": tick})

	var b strings.Builder
	fmt.Fprintf(&b, "the field at or before tick %d:\n", tick)
	for _, r := range rows {
		kind := "units"
		if r.Building {
			kind = "buildings"
		}
		if r.Remembered {
			kind += " (remembered, not currently visible)"
		}
		fmt.Fprintf(&b, "%s %s: %d\n", r.Side, kind, r.N)
	}
	switch {
	case ever.N == 0:
		b.WriteString(fieldAtAbsentFeedNote)
	case th.Zones == 0:
		fmt.Fprintf(&b, "threat map: 0 zones here, though %d rows exist elsewhere in this game, so the feed was running. Nothing had been scouted at this point: the approach router saw every corridor as clear, which is ignorance rather than safety.\n", ever.N)
	default:
		fmt.Fprintf(&b, "threat map: %d zones carrying danger, peak %.2f.\n", th.Zones, th.Peak)
	}
	return b.String()
}

func (iv *investigator) compareGames(ctx context.Context) string {
	if iv.ch == nil {
		return "no ClickHouse configured."
	}
	type row struct {
		ID      int64   `json:"id"`
		Ticks   int64   `json:"ticks"`
		Killed  int64   `json:"bk"`
		PerTick float64 `json:"per_tick"`
		Trade   float64 `json:"trade"`
	}
	rows, err := ch.Query[row](ctx, iv.ch,
		`SELECT id, duration_ticks AS ticks, engine_buildings_killed AS bk,
		        round(engine_earned / duration_ticks, 2) AS per_tick,
		        round(engine_kills_cost / engine_deaths_cost, 2) AS trade
		 FROM games WHERE duration_ticks > 0 ORDER BY id DESC LIMIT 12`, nil)
	if err != nil || len(rows) == 0 {
		return "the cross-game table is not loaded in ClickHouse; the per-game facts above still stand."
	}
	var b strings.Builder
	b.WriteString("recent games. per_tick is credits earned per tick; it has predicted the outcome in every game measured, and a game below about 1.2 did not lose on tactics.\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "game %d: %d ticks, %d enemy buildings destroyed, %.2f credits/tick, trade %.2f\n",
			r.ID, r.Ticks, r.Killed, r.PerTick, r.Trade)
	}
	return b.String()
}

// settleWindow is how long a session's stream must be quiet before its
// telemetry counts as complete.
//
// The shipper moves sealed segments every few seconds, and the last of them
// land after the game has already archived - so "the game is over" and "the
// data is all here" are not the same moment. Sixty seconds is comfortably more
// than a ship cycle and costs only a re-run if it is wrong.
const settleWindow = 60

// settled reports whether a game is finished AND its telemetry has stopped
// arriving, which is the only state in which an investigation is worth keeping.
//
// A live game is deliberately never cached: it would freeze a reading of half a
// game and serve it forever, and a second look at more data is a different and
// probably better answer. A finished one is answered once.
func (iv *investigator) settled(ctx context.Context, session string) bool {
	if iv.ch == nil || session == "" {
		// No stream to settle. The replay facts are from the archive, which is
		// written at game end, so the reading is as final as it will get.
		return true
	}
	type row struct {
		Quiet int `json:"quiet"`
	}
	r, err := ch.One[row](ctx, iv.ch,
		`SELECT toInt32(dateDiff('second', max(ingested_at), now())) AS quiet
		 FROM stream_segments WHERE session_id = {session:String}`,
		map[string]any{"session": session})
	if err != nil {
		// Unknown is not settled: caching on a failed check is how a partial
		// reading becomes permanent.
		return false
	}
	return r.Quiet >= settleWindow
}

// rawQuery runs the model's own SELECT.
//
// The guards are the database's, not a regex over the SQL: the client connects
// readonly=2, which permits SELECT and refuses everything that writes, and caps
// both the rows returned and the rows scanned. A string check would be one
// clever encoding away from useless; the server-side setting is not.
//
// Results come back as generic rows because the shape is whatever was asked
// for. An error is returned to the model rather than swallowed - a failed query
// it can see is a query it can fix, and it has turns to spare for that.
func (iv *investigator) rawQuery(ctx context.Context, session, sql string) string {
	if iv.ch == nil {
		return "no ClickHouse configured."
	}
	if strings.TrimSpace(sql) == "" {
		return "empty query."
	}
	rows, err := ch.Query[map[string]any](ctx, iv.ch, sql, map[string]any{"session": session})
	if err != nil {
		return "query failed: " + err.Error() + "\nCheck the column names against the schema above, and remember the connection is read-only."
	}
	if len(rows) == 0 {
		return "no rows. That is an answer: the thing asked about did not happen, or the filter excluded it."
	}

	// Column order is not stable across a map, so take it from the first row
	// and sort it: an unstable header makes two runs of the same query look
	// like different results.
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}
	sort.Strings(cols)

	var b strings.Builder
	fmt.Fprintf(&b, "%d rows.\n", len(rows))
	b.WriteString(strings.Join(cols, " | "))
	b.WriteString("\n")
	for _, r := range rows {
		parts := make([]string, 0, len(cols))
		for _, c := range cols {
			parts = append(parts, fmt.Sprint(r[c]))
		}
		b.WriteString(strings.Join(parts, " | "))
		b.WriteString("\n")
	}
	return b.String()
}
