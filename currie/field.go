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
	Ticks   []int // the scrub positions, coarse
	PrevURL string
	NextURL string
	Markers []fieldMarker
	Ours    int
	Enemy   int
	Note    string

	// Side of the square viewport, in SVG units.
	Size float64
}

// fieldSize is the rendered square. The map is square in every game recorded so
// far (91x91), and a fixed viewport keeps the scrub from jumping.
const fieldSize = 720

// fieldScrubStride is how far apart the scrub positions are. State arrives
// every 10 ticks and units are sampled every 20, so 500 is roughly a
// twenty-second step: coarse enough to cross a long game in a few clicks.
const fieldScrubStride = 500

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
	for t := ext.Lo; t <= ext.Hi; t += fieldScrubStride {
		v.Ticks = append(v.Ticks, t)
	}
	if v.Tick <= 0 {
		v.Tick = ext.Lo
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

	v.PrevURL = fieldURL(session, v.Tick-fieldScrubStride)
	v.NextURL = fieldURL(session, v.Tick+fieldScrubStride)
	return v
}

func fieldURL(session string, tick int) string {
	if tick < 0 {
		tick = 0
	}
	return "/field/" + session + "/frame?tick=" + strconv.Itoa(tick)
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
