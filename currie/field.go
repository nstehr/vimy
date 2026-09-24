package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/nstehr/vimy/currie/ch"
)

// The field, as Vimy saw it.
//
// Everything this project has diagnosed so far came from scalars: spread,
// members, a distance ratio, a blocker count. They are summaries, and summaries
// were read wrong more than once - a squad "arriving" that was two stragglers,
// a sawtooth attributed to three different rules before the right one, an
// approach that looked like bad routing and was missing intel. A scalar can be
// argued with. A picture of where everything stood cannot.
//
// Rendered server-side as SVG. The grid is a few hundred shapes, the template
// does no arithmetic, and a scrub is one fragment swap - the same shape as the
// live panel, for the same reasons.

// fieldMarker is one actor drawn on the map, already in SVG coordinates.
type fieldMarker struct {
	X, Y  float64
	R     float64
	Class string // ours / enemy, building / unit, plus a role hint
	Label string // hover text: type and id
}

type fieldView struct {
	Session string
	Tick    int

	// The timeline, as a range input rather than a row of buttons: a long game
	// is 200 scrub positions and clicking through them is not scrubbing.
	Lo, Hi, Stride int

	// Live follows the newest sample and keeps polling. Dragging the timeline
	// pins a tick and stops it, because a frame that jumps out from under the
	// cursor cannot be read.
	Live bool
	Poll string

	Markers []fieldMarker
	Ours    int
	Enemy   int
	Note    string

	// Side of the square viewport, in SVG units.
	Size float64
}

// fieldPoll is how often a live field asks again. Slower than the live panel:
// a frame is a few hundred shapes and the game moves 20 ticks between samples,
// so a faster poll redraws the same field.
const fieldPoll = "3s"

// fieldSize is the rendered square. The map is square in every game recorded so
// far (91x91), and a fixed viewport keeps the scrub from jumping.
const fieldSize = 720

// fieldScrubStride is the timeline's step. Units are sampled every 20 ticks and
// the loader snaps to the nearest sample at or before the requested tick, so
// this only has to be fine enough that dragging feels continuous.
const fieldScrubStride = 20

type unitRow struct {
	Tick     int    `json:"tick"`
	UnitID   int    `json:"unit_id"`
	Type     string `json:"type"`
	Side     string `json:"side"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	HP       int    `json:"hp"`
	Idle     bool   `json:"idle"`
	Building bool   `json:"is_building"`
}

// markerClass picks the visual weight. Harvesters and buildings read
// differently from combat units because the question is nearly always "where
// was the army", and an economy that fills the screen hides it.
func markerClass(r unitRow) (class string, radius float64) {
	side := "enemy"
	if r.Side == "ours" {
		side = "ours"
	}
	switch {
	case r.Building:
		return side + " building", 5
	case strings.HasPrefix(strings.ToLower(r.Type), "harv"):
		return side + " harv", 3
	default:
		return side + " unit", 3.5
	}
}

func loadField(ctx context.Context, c *ch.Client, session string, tick int) *fieldView {
	v := &fieldView{Session: session, Tick: tick, Size: fieldSize}
	if c == nil {
		v.Note = "no ClickHouse configured"
		return v
	}

	// The session's extent, so the scrubber covers the game rather than
	// whatever happens to have been shipped first.
	type extent struct {
		Lo int `json:"lo"`
		Hi int `json:"hi"`
	}
	ext, err := ch.One[extent](ctx, c,
		`SELECT min(tick) AS lo, max(tick) AS hi FROM stream_units WHERE session_id = {session:String}`,
		map[string]any{"session": session})
	if err != nil || ext.Hi == 0 {
		v.Note = "no unit telemetry for this session yet"
		return v
	}
	v.Lo, v.Hi, v.Stride = ext.Lo, ext.Hi, fieldScrubStride
	// tick <= 0 means "wherever the game is now", which is also what a live
	// frame asks for on every poll.
	if v.Tick <= 0 {
		v.Tick = ext.Hi
		v.Live = true
		v.Poll = fieldPoll
	}

	// The sample at or just before the requested tick, so a scrub position
	// between samples shows the last known field rather than an empty one.
	rows, err := ch.Query[unitRow](ctx, c,
		`SELECT tick, unit_id, type, side, x, y, hp, idle, is_building
		 FROM stream_units
		 WHERE session_id = {session:String}
		   AND tick = (SELECT max(tick) FROM stream_units
		               WHERE session_id = {session:String} AND tick <= {tick:UInt32})`,
		map[string]any{"session": session, "tick": v.Tick})
	if err != nil {
		slog.Warn("field units", "session", session, "error", err)
		v.Note = "query failed: " + err.Error()
		return v
	}

	// Map coordinates to the viewport. Scaled by the observed extent rather
	// than an assumed map size: the field is what was seen, and nothing here
	// knows how big the map was.
	maxXY := 1
	for _, r := range rows {
		if r.X > maxXY {
			maxXY = r.X
		}
		if r.Y > maxXY {
			maxXY = r.Y
		}
	}
	scale := fieldSize / float64(maxXY+4)

	for _, r := range rows {
		class, radius := markerClass(r)
		if r.Side == "ours" {
			v.Ours++
		} else {
			v.Enemy++
		}
		v.Markers = append(v.Markers, fieldMarker{
			X: float64(r.X) * scale, Y: float64(r.Y) * scale, R: radius,
			Class: class,
			Label: fmt.Sprintf("%s #%d (%d,%d) hp %d", r.Type, r.UnitID, r.X, r.Y, r.HP),
		})
	}

	return v
}

func (s *server) handleField(w http.ResponseWriter, r *http.Request) {
	session := r.PathValue("session")
	tick, _ := strconv.Atoi(r.URL.Query().Get("tick"))
	s.render(w, "field.html.tmpl", loadField(r.Context(), s.ch, session, tick))
}

// handleFieldFrame serves the fragment a scrub swaps in.
func (s *server) handleFieldFrame(w http.ResponseWriter, r *http.Request) {
	session := r.PathValue("session")
	tick, _ := strconv.Atoi(r.URL.Query().Get("tick"))
	s.render(w, "fieldframe", loadField(r.Context(), s.ch, session, tick))
}
