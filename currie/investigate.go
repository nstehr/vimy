package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

func (iv *investigator) Investigate(ctx context.Context, r *Replay) (*Insight, []TraceStep, error) {
	f := facts(r)
	session := iv.sessionFor(ctx, r.Game.ID)

	var history []types.AgentMessage
	var trace []TraceStep

	for step := range maxInvestigationSteps {
		res, err := baml_client.InvestigateGameStep(ctx, f, toolMenu, history)
		if err != nil {
			return nil, trace, fmt.Errorf("investigate step %d: %w", step, err)
		}

		if final := res.AsFinalInsightTool(); final != nil {
			ins := &Insight{Summary: final.Summary, Suggestion: final.Suggestion, Caveat: final.Caveat}
			if a := final.Directive_advice; a != nil {
				ins.Directive = &DirectiveAdvice{Diagnosis: a.Diagnosis, Revision: a.Revision, Confidence: a.Confidence}
			}
			for _, fd := range final.Findings {
				ins.Findings = append(ins.Findings, Finding{Claim: fd.Claim, Evidence: fd.Evidence, Confidence: fd.Confidence})
			}
			return ins, trace, nil
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
	return nil, trace, fmt.Errorf("no conclusion after %d steps", maxInvestigationSteps)
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

compare_games — the same headline numbers across recent games, to tell what is
  particular to this one from what is true of the run. Income per tick has
  predicted the outcome in every game measured.
`

// run executes whichever tool the model picked. Returns the tool name, the
// model's stated reason, and the result to feed back.
func (iv *investigator) run(ctx context.Context, session string, res types.Union5CompareGamesToolOrFieldAtToolOrFinalInsightToolOrSquadTimelineToolOrStrikeBlockersTool) (string, string, string) {
	switch {
	case res.AsSquadTimelineTool() != nil:
		return "squad_timeline", res.AsSquadTimelineTool().Reason, iv.squadTimeline(ctx, session)
	case res.AsStrikeBlockersTool() != nil:
		return "strike_blockers", res.AsStrikeBlockersTool().Reason, iv.strikeBlockers(ctx, session)
	case res.AsFieldAtTool() != nil:
		t := res.AsFieldAtTool()
		return "field_at", t.Reason, iv.fieldAt(ctx, session, int(t.Tick))
	case res.AsCompareGamesTool() != nil:
		return "compare_games", res.AsCompareGamesTool().Reason, iv.compareGames(ctx)
	}
	return "", "", ""
}

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
	type trow struct {
		Zones int     `json:"zones"`
		Peak  float64 `json:"peak"`
	}
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
	fmt.Fprintf(&b, "threat map: %d zones carrying danger, peak %.2f. ", th.Zones, th.Peak)
	if th.Zones == 0 {
		b.WriteString("An empty threat map means nothing had been scouted, so the approach router saw every corridor as clear. That is ignorance, not safety.\n")
	} else {
		b.WriteString("\n")
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
