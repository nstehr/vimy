package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

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

// fieldZone is one terrain cell of the background.
type fieldZone struct {
	X, Y, W, H float64
	Class      string
	// Opacity is set only for the threat overlay, where intensity is the point:
	// where the danger is concentrated, not merely where it is non-zero.
	Opacity string
}

type fieldView struct {
	Session string
	Tick    int

	// The danger map the approach router actually reads. An empty one is the
	// finding, not a gap: it means nothing has been scouted and every corridor
	// scores clear.
	Threat    []fieldZone
	ThreatMax float64

	// The ground. The sidecar has had this grid since it gained terrain
	// awareness and nothing ever drew it, so every map of a game so far has
	// been units floating on a blank square.
	Zones []fieldZone

	// The timeline, as a range input rather than a row of buttons: a long game
	// is 200 scrub positions and clicking through them is not scrubbing.
	Lo, Hi, Stride int

	// Live follows the newest sample and keeps polling. Dragging the timeline
	// pins a tick and stops it, because a frame that jumps out from under the
	// cursor cannot be read.
	Live bool
	Poll string

	// Playback, as a property of the frame rather than of the server or the
	// browser. Each frame asks for the next one after a delay, so the whole
	// state is the URL that produced this frame: nothing to get out of sync,
	// nothing to clean up when a tab closes, and a reload resumes exactly
	// where it was. Pausing is just a request without the flag.
	Playing   bool
	NextTick  int
	PlayDelay string
	// PlayFrom is where the play control starts. The tick in hand while there
	// is somewhere left to go, and the beginning once the timeline has run
	// out -- so the button at the end of a game reads as replay rather than as
	// a control that does nothing.
	PlayFrom int

	Markers  []fieldMarker
	Ours     int
	Enemy    int
	Believed int
	Neutral  int
	Note     string

	// Side of the square viewport, in SVG units.
	Size float64
	// TerrainSpan is the map width the terrain grid covers, so markers can be
	// scaled to the same ground rather than to whatever they happen to span.
	TerrainSpan float64
}

// fieldPoll is how often a live field asks again. Slower than the live panel:
// a frame is a few hundred shapes and the game moves 20 ticks between samples,
// so a faster poll redraws the same field.
const fieldPoll = "3s"

// fieldSize is the rendered square. The map is square in every game recorded so
// far (91x91), and a fixed viewport keeps the scrub from jumping.
const fieldSize = 720

// Playback pacing. A step per frame rather than a sample per frame: game 174
// is 2,820 samples, and at any watchable frame rate one sample per frame is a
// six-minute replay of a six-minute game. The step is sized so a game of any
// length takes about the same time to watch, which is what makes two games
// comparable by eye.
const (
	playDelay = "150ms"
	// Frames in a full replay: about 45 seconds at the delay above.
	playFrames = 300
)

// playStep is how far one frame advances. Never finer than the sampling
// stride, because there is nothing in between to show.
func playStep(lo, hi int) int {
	if span := hi - lo; span/playFrames > fieldScrubStride {
		return (span / playFrames / fieldScrubStride) * fieldScrubStride
	}
	return fieldScrubStride
}

// fieldScrubStride is the timeline's step. Units are sampled every 20 ticks and
// the loader snaps to the nearest sample at or before the requested tick, so
// this only has to be fine enough that dragging feels continuous.
const fieldScrubStride = 20

type unitRow struct {
	Tick       int    `json:"tick"`
	UnitID     int    `json:"unit_id"`
	Type       string `json:"type"`
	Side       string `json:"side"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	HP         int    `json:"hp"`
	Idle       bool   `json:"idle"`
	Building   bool   `json:"is_building"`
	Remembered bool   `json:"remembered"`
}

// markerClass picks the visual weight. Harvesters and buildings read
// differently from combat units because the question is nearly always "where
// was the army", and an economy that fills the screen hides it.
func markerClass(r unitRow) (class string, radius float64) {
	side := "enemy"
	switch r.Side {
	case "ours":
		side = "ours"
	case "neutral":
		// Objectives rather than combatants: drawn as their own thing so they
		// never read as either army's.
		return "neutral", 5.5
	}
	switch {
	case r.Remembered:
		// Belief, not sight: drawn hollow so it never reads as a live sighting.
		return side + " remembered", 6
	case r.Building:
		return side + " building", 5
	case strings.HasPrefix(strings.ToLower(r.Type), "harv"):
		return side + " harv", 3
	default:
		return side + " unit", 3.5
	}
}

func loadField(ctx context.Context, c *ch.Client, session string, tick int, playing bool) *fieldView {
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
	v.loadTerrain(ctx, c, session)
	v.loadThreat(ctx, c, session, tick)
	v.Lo, v.Hi, v.Stride = ext.Lo, ext.Hi, fieldScrubStride

	v.clock(ext.Lo, ext.Hi, playing)

	// The sample at or just before the requested tick, so a scrub position
	// between samples shows the last known field rather than an empty one.
	rows, err := ch.Query[unitRow](ctx, c,
		`SELECT tick, unit_id, type, side, x, y, hp, idle, is_building, remembered
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
	// Prefer the terrain's own span: scaling markers to their observed extent
	// makes the field breathe as units move, and puts them out of register with
	// the ground. Fall back to the extent when a session predates the grid.
	scale := fieldSize / float64(maxXY+4)
	if v.TerrainSpan > 0 {
		scale = fieldSize / v.TerrainSpan
	}

	for _, r := range rows {
		class, radius := markerClass(r)
		switch {
		case r.Remembered:
			v.Believed++
		case r.Side == "ours":
			v.Ours++
		case r.Side == "neutral":
			v.Neutral++
		default:
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
	s.render(w, "field.html.tmpl", s.field(r))
}

// clock settles which tick this frame shows and what it does next.
//
// Pure, and separate from the queries, because it is the only part of the page
// with a state machine in it: live follows the present, playback walks the
// past, and a frame that tried to do both would jump to the newest sample
// mid-replay.
func (v *fieldView) clock(lo, hi int, playing bool) {
	switch {
	case playing:
		if v.Tick < lo {
			v.Tick = lo
		}
		v.PlayDelay = playDelay
		v.NextTick = min(v.Tick+playStep(lo, hi), hi)
		// The last frame stops instead of asking forever for a tick it is
		// already showing. The control then reads "replay", and starts from
		// the beginning, because that is the only direction left.
		v.Playing = v.Tick < hi
	// tick <= 0 means "wherever the game is now", which is also what a live
	// frame asks for on every poll.
	case v.Tick <= 0:
		v.Tick = hi
		v.Live = true
		v.Poll = fieldPoll
	}
	v.PlayFrom = v.Tick
	if v.PlayFrom >= hi {
		v.PlayFrom = lo
	}
}

// handleFieldFrame serves the fragment a scrub swaps in, and that playback
// asks for again on a delay.
func (s *server) handleFieldFrame(w http.ResponseWriter, r *http.Request) {
	s.render(w, "fieldframe", s.field(r))
}

// field reads one frame's request. Both handlers take the same parameters, so
// a link into the middle of a replay is an ordinary URL.
func (s *server) field(r *http.Request) *fieldView {
	tick, _ := strconv.Atoi(r.URL.Query().Get("tick"))
	return loadField(r.Context(), s.ch, r.PathValue("session"), tick, r.URL.Query().Get("play") == "1")
}

// terrainClass maps the encoded grid to a style. Bridges get their own because
// they are the chokepoints - a land corridor over water is where an assault can
// actually be stopped, and seeing one explains a defence line that otherwise
// looks arbitrary.
func terrainClass(c byte) string {
	switch c {
	case '~':
		return "water"
	case '#':
		return "cliff"
	case '=':
		return "bridge"
	default:
		return "land"
	}
}

type terrainRow struct {
	Cols  int    `json:"terrain_cols"`
	Rows  int    `json:"terrain_rows"`
	CellW int    `json:"terrain_cell_w"`
	CellH int    `json:"terrain_cell_h"`
	Grid  string `json:"terrain"`
}

// terrainCache holds each session's grid for the life of the process.
//
// The grid is fixed at the hello handshake and cannot change during a game, so
// re-reading it was pure waste - and worse than waste. A live field polls every
// three seconds, every poll re-queried it, and a single failed query blanked
// the ground: terrain present at tick 1100 and gone at 1360, on the same
// session, because the error was swallowed silently and the page drew what it
// had. Fetching once removes the failure window rather than narrowing it.
var terrainCache sync.Map // session id -> terrainRow

// loadTerrain paints the background. Silent when a session predates the grid
// being recorded: an older game still draws, just on blank ground.
func (v *fieldView) loadTerrain(ctx context.Context, c *ch.Client, session string) {
	var t terrainRow
	if hit, ok := terrainCache.Load(session); ok {
		t = hit.(terrainRow)
	} else {
		var err error
		// Ordered and filtered rather than LIMIT 1 on its own: the shipper
		// inserts a session row per cycle, so several can exist at once until
		// they merge, and an unordered pick is a coin flip between them.
		t, err = ch.One[terrainRow](ctx, c,
			`SELECT terrain_cols, terrain_rows, terrain_cell_w, terrain_cell_h, terrain
			 FROM stream_sessions
			 WHERE session_id = {session:String} AND terrain != ''
			 ORDER BY started_at DESC LIMIT 1`,
			map[string]any{"session": session})
		if err != nil {
			// Logged, not swallowed. A blank map that says nothing is how this
			// looked like a rendering glitch rather than a failed query.
			slog.Warn("field terrain", "session", session, "error", err)
			return
		}
		if t.Cols > 0 {
			terrainCache.Store(session, t)
		}
	}
	if t.Cols <= 0 || len(t.Grid) < t.Cols*t.Rows {
		return
	}
	// Zones are scaled to the same viewport the markers use, which is sized
	// from the observed unit extent - so the ground and the actors agree even
	// though neither knows the map dimensions.
	span := float64(t.Cols * t.CellW)
	if span <= 0 {
		return
	}
	zw := fieldSize / float64(t.Cols)
	zh := fieldSize / float64(t.Rows)
	v.TerrainSpan = span
	for row := 0; row < t.Rows; row++ {
		for col := 0; col < t.Cols; col++ {
			class := terrainClass(t.Grid[row*t.Cols+col])
			if class == "land" {
				continue // the default ground; drawing it is 1024 wasted rects
			}
			v.Zones = append(v.Zones, fieldZone{
				X: float64(col) * zw, Y: float64(row) * zh, W: zw + .5, H: zh + .5,
				Class: class,
			})
		}
	}
}

type threatCell struct {
	Col   int     `json:"col"`
	Row   int     `json:"row"`
	Value float64 `json:"value"`
}

// loadThreat paints the danger map for the nearest sample at or before the
// tick. Opacity is scaled to the frame's own maximum rather than an absolute:
// the interesting question is where the danger is concentrated, and an absolute
// scale makes an early game with one sighting look identical to a blank one.
func (v *fieldView) loadThreat(ctx context.Context, c *ch.Client, session string, tick int) {
	if v.TerrainSpan <= 0 {
		return
	}
	cells, err := ch.Query[threatCell](ctx, c,
		`SELECT col, row, value FROM stream_threat
		 WHERE session_id = {session:String}
		   AND tick = (SELECT max(tick) FROM stream_threat
		               WHERE session_id = {session:String} AND tick <= {tick:UInt32})`,
		map[string]any{"session": session, "tick": tick})
	if err != nil || len(cells) == 0 {
		return
	}
	for _, c := range cells {
		if c.Value > v.ThreatMax {
			v.ThreatMax = c.Value
		}
	}
	// Zone size comes from the terrain grid, which shares this geometry.
	zw := fieldSize / float64(threatCols)
	zh := fieldSize / float64(threatRows)
	for _, cell := range cells {
		// Relative to this frame's own maximum, not an absolute: an early game
		// with a single sighting would otherwise be indistinguishable from a
		// blank field, and that distinction is the whole reason to draw it.
		share := cell.Value / v.ThreatMax
		v.Threat = append(v.Threat, fieldZone{
			X: float64(cell.Col) * zw, Y: float64(cell.Row) * zh, W: zw + .5, H: zh + .5,
			Class:   "threat",
			Opacity: strconv.FormatFloat(0.10+0.45*share, 'f', 3, 64),
		})
	}
}

// The threat field is rastered onto the terrain grid, which is fixed at 32x32
// whatever the map size.
const (
	threatCols = 32
	threatRows = 32
)
