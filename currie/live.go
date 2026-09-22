package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/nstehr/vimy/currie/ch"
)

// The game as it is being played.
//
// Vimy's own dashboard shows INTENT -- the directive, the doctrine currently
// compiled, the weights over time, snapshots the agent holds in memory. None
// of that says what the squads actually did. This is the other half: rallies,
// blocked strikes and transit samples, one row per occurrence, arriving while
// the game runs.
//
// It lives in Currie rather than in vimy-core on purpose. The sidecar must not
// grow a reason to talk to a database -- that constraint is what the whole
// write-ahead log exists to honour -- and Vimy's server should not gain a
// ClickHouse dependency to render a panel.

// liveLag is how stale the newest shipped segment may be before the page stops
// calling a session live.
//
// Generous against `-ship-every 5s`: the shipper only moves SEALED segments,
// so a quiet stretch is normal and a game that fell behind by a few segments
// has not ended. Calling a running game finished is the worse error.
const liveLag = 2 * time.Minute

// livePoll matches the shipper's default interval. Polling faster re-renders
// the same rows and asks ClickHouse to prove it.
const livePoll = "5s"

// transitBucket groups transit samples for the trajectory. Ticks, not seconds:
// the stream has no clock and the tick is the only ordering the sidecar and
// the engine agree on.
const transitBucket = 5000

// transitSpan caps the trajectory at the most recent buckets, so a marathon
// game does not render a sparkline a thousand points wide.
const transitSpan = 40

type liveView struct {
	// Poll is where the fragment asks again, and how often.
	Poll string
	URL  string

	Session *StreamSession
	// Live is false when the newest segment is older than liveLag: the panel
	// then reads as the last session rather than a current one, which is a
	// different claim and has to look like one.
	Live bool
	Lag  string
	Tick int64

	Rally   *CohortStats
	Strikes []StrikeRow
	Transit []TransitPoint
	Rates   []FireRate

	// The trajectory as a polyline, built here rather than in the template:
	// the template has no arithmetic and a sparkline is all arithmetic.
	Spark Spark

	Note string
}

// StrikeRow is one reason a strike did not happen, and how often.
//
// strike_blocked_no_target had to be SPLIT in two because one counter hid two
// causes with opposite fixes, and that split could not be applied backwards. A
// row per occurrence cannot hide anything: the next cause is a new `reason`
// value here, not a migration.
type StrikeRow struct {
	Reason   string `json:"reason"`
	Blocked  int64  `json:"blocked"`
	LastTick int64  `json:"last_tick"`

	// Share of the largest blocker, for the bar. Computed here rather than in
	// the template, which has no arithmetic.
	Share float64 `json:"-"`
}

// TransitPoint is one window of the squad's approach.
//
// The in-memory sampler computes the distance to the target as a fraction of
// the map diagonal and then throws the number away, bucketing into three bands
// and summing -- game 148's 118,920 ticks reduce to 76 far / 5 mid / 0 near.
// That cannot say WHEN the squad stopped closing. This can: a squad that
// closes and then stops shows a floor in Closest that never falls further.
type TransitPoint struct {
	Tick     int64   `json:"tick_bucket"`
	Samples  int64   `json:"samples"`
	Closest  float64 `json:"closest"`
	Mean     float64 `json:"mean_fraction"`
	Spread   float64 `json:"mean_spread"`
	Cohesion float64 `json:"cohesion"`
}

// Spark is a trajectory ready for an <svg>.
//
// Both series are drawn on the same 0..1 axis because target_fraction already
// is one — no normalising, so two sparklines from two different games are
// directly comparable by eye. A chart that rescales to its own data hides
// exactly the flattening this is drawn to show.
type Spark struct {
	Mean    string
	Closest string
	// Floor is the lowest fraction reached: how close the squad ever got.
	Floor float64
	// Stalled marks a trajectory whose closest approach stopped improving over
	// the last third of the samples. A fact about the line, not a diagnosis.
	Stalled bool
	Width   int
	Height  int
}

func (v *liveView) Has() bool { return v != nil && v.Session != nil }

const latestSessionSQL = `
SELECT
	session_id,
	toUnixTimestamp(max(ingested_at)) AS last_ingest,
	toInt64(count())                  AS segments
FROM stream_segments FINAL
GROUP BY session_id
ORDER BY last_ingest DESC
LIMIT 1`

const sessionByIDSQL = `
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
WHERE session_id = {session:String}`

const liveTickSQL = `
SELECT toInt64(max(tick)) AS at_tick FROM stream_rule_evals WHERE ` + filterForSession

const strikesSQL = `
SELECT
	reason,
	toInt64(count())   AS blocked,
	toInt64(max(tick)) AS last_tick
FROM stream_events
WHERE kind = 'strike-blocked' AND ` + filterForSession + `
GROUP BY reason
ORDER BY blocked DESC`

const transitSQL = `
SELECT
	toInt64(intDiv(tick, {bucket:UInt32}) * {bucket:UInt32})  AS tick_bucket,
	toInt64(count())                                          AS samples,
	round(min(attrs['target_fraction']), 3)                   AS closest,
	round(avg(attrs['target_fraction']), 3)                   AS mean_fraction,
	round(avg(spread), 1)                                     AS mean_spread,
	round(ifNotFinite(avg(near) / avg(members), 0), 2)        AS cohesion
FROM stream_events
WHERE kind = 'transit' AND ` + filterForSession + `
GROUP BY tick_bucket
ORDER BY tick_bucket`

// liveSession finds the session the shipper most recently moved anything for.
//
// Keyed off the ledger rather than stream_sessions.started_at: the newest
// session by start time may be one that ended an hour ago while an older one
// is still being played, and the question here is which game is moving.
func liveSession(ctx context.Context, c *ch.Client) (*StreamSession, time.Time, error) {
	latest, err := ch.One[struct {
		SessionID  string `json:"session_id"`
		LastIngest int64  `json:"last_ingest"`
		Segments   int64  `json:"segments"`
	}](ctx, c, latestSessionSQL, nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	sess, err := ch.One[StreamSession](ctx, c, sessionByIDSQL, map[string]any{"session": latest.SessionID})
	if err != nil {
		return nil, time.Time{}, err
	}
	sess.LastIngest, sess.Segments = latest.LastIngest, latest.Segments
	return &sess, time.Unix(latest.LastIngest, 0), nil
}

// loadLive assembles the panel. Every section is best-effort, as on the report:
// a failed transit query costs the trajectory and nothing else.
func loadLive(ctx context.Context, c *ch.Client, topRates int) *liveView {
	v := &liveView{Poll: livePoll, URL: "/live/panel"}
	if c == nil {
		v.Note = "no ClickHouse configured — start it with `make -C clickhouse up` and ship with `currie -ship -ship-every 5s`"
		return v
	}

	sess, last, err := liveSession(ctx, c)
	if err != nil {
		if errors.Is(err, ch.ErrNoRows) {
			v.Note = "nothing shipped yet — run `currie -ship -ship-every 5s` while a game is running"
		} else {
			v.Note = "ClickHouse could not be reached: " + err.Error()
		}
		return v
	}
	v.Session = sess
	lag := time.Since(last)
	v.Live = lag < liveLag
	v.Lag = humanLag(lag)
	if !v.Live {
		v.Note = "no segment has landed recently — this is the last session, not a running game"
	}

	args := map[string]any{"session": sess.SessionID}
	if tick, err := ch.One[struct {
		Tick int64 `json:"at_tick"`
	}](ctx, c, liveTickSQL, args); err == nil {
		v.Tick = tick.Tick
	}

	if rally, err := ch.One[CohortStats](ctx, c, ralliesSQL(filterForSession), args); err == nil && rally.Rallies > 0 {
		v.Rally = &rally
	}
	if strikes, err := ch.Query[StrikeRow](ctx, c, strikesSQL, args); err != nil {
		slog.Warn("live strikes", "session", sess.SessionID, "error", err)
	} else {
		var top int64
		for _, st := range strikes {
			if st.Blocked > top {
				top = st.Blocked
			}
		}
		for i := range strikes {
			if top > 0 {
				strikes[i].Share = float64(strikes[i].Blocked) / float64(top)
			}
		}
		v.Strikes = strikes
	}

	targs := map[string]any{"session": sess.SessionID, "bucket": transitBucket}
	if transit, err := ch.Query[TransitPoint](ctx, c, transitSQL, targs); err != nil {
		slog.Warn("live transit", "session", sess.SessionID, "error", err)
	} else {
		if len(transit) > transitSpan {
			transit = transit[len(transit)-transitSpan:]
		}
		v.Transit = transit
		v.Spark = sparkline(transit)
	}

	if rates, err := ch.Query[FireRate](ctx, c, fireRatesSQL(filterForSession), args); err != nil {
		slog.Warn("live rates", "session", sess.SessionID, "error", err)
	} else {
		if len(rates) > topRates {
			rates = rates[:topRates]
		}
		v.Rates = rates
	}
	return v
}

// sparkline projects the trajectory onto a fixed 0..1 axis.
//
// Fixed, not fitted: target_fraction is already a fraction of the map diagonal,
// so an unscaled line means the same thing in every game and two of them can be
// compared by eye. Fitting each line to its own range would make a squad that
// never left the base look identical to one that crossed the map.
func sparkline(points []TransitPoint) Spark {
	s := Spark{Width: 720, Height: 120}
	if len(points) < 2 {
		return s
	}
	var mean, closest []string
	floor := 1.0
	for i, p := range points {
		x := float64(i) / float64(len(points)-1) * float64(s.Width)
		mean = append(mean, fmt.Sprintf("%.1f,%.1f", x, (1-clamp01(p.Mean))*float64(s.Height)))
		closest = append(closest, fmt.Sprintf("%.1f,%.1f", x, (1-clamp01(p.Closest))*float64(s.Height)))
		if p.Closest < floor {
			floor = p.Closest
		}
	}
	s.Mean = strings.Join(mean, " ")
	s.Closest = strings.Join(closest, " ")
	s.Floor = floor

	// Stalled: the closest approach over the last third is no better than the
	// best already reached before it. Stated as a fact about the line, because
	// a squad holding position under orders looks the same from here.
	cut := len(points) * 2 / 3
	if cut > 0 && cut < len(points) {
		best := 1.0
		for _, p := range points[:cut] {
			if p.Closest < best {
				best = p.Closest
			}
		}
		later := 1.0
		for _, p := range points[cut:] {
			if p.Closest < later {
				later = p.Closest
			}
		}
		s.Stalled = later >= best
	}
	return s
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func humanLag(d time.Duration) string {
	switch {
	case d < 2*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// handleLive serves the page frame. The frame never changes; the panel inside
// it polls.
func (s *server) handleLive(w http.ResponseWriter, r *http.Request) {
	s.render(w, "live.html.tmpl", loadLive(r.Context(), s.ch, 14))
}

// handleLivePanel serves the fragment htmx swaps in.
func (s *server) handleLivePanel(w http.ResponseWriter, r *http.Request) {
	s.render(w, "livepanel", loadLive(r.Context(), s.ch, 14))
}
