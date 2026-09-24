package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nstehr/vimy/vimy-core/ipc"
	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/store"
	"github.com/nstehr/vimy/vimy-core/wal"
)

// Agent owns the decision-making for a single player session.
type Agent struct {
	Conn       *ipc.Connection
	Player     string
	Faction    string
	Engine     *rules.Engine
	Strategist *Strategist
	Store      *store.Store
	// terrain is kept so the session record can carry it: static for the whole
	// game, written down once rather than sampled.
	terrain *model.TerrainGrid

	// WAL is the telemetry log for this game, opened at Hello and nil until
	// then. Sealed once the retrospective knows which archive row the game
	// became.
	WAL *wal.Log
	// Telemetry is how to open that log. Nil when streaming is off.
	Telemetry *TelemetryConfig
	ctx       context.Context

	// When a game state was last processed, for the stall watchdog.
	lastState atomic.Int64
}

// TelemetryConfig is everything needed to open a game's log, carried rather
// than an opened log: a connection that never plays must not leave a session
// directory behind, and only one connection may stream at a time.
type TelemetryConfig struct {
	Dir         string
	NewID       func() string
	RulesDigest string
	Revision    string
	Modified    bool
}

// startTelemetry claims the engine's sink for this game, if streaming is on and
// nothing else holds it. Losing the claim is not an error: it means another
// connection is already playing, and two logs against one engine would leave
// the loser sealing an empty session.
func (a *Agent) startTelemetry() {
	if a.Telemetry == nil || a.WAL != nil {
		return
	}
	cfg := a.Telemetry
	sink, err := a.Engine.AttachEvents(func() (rules.TelemetrySink, error) {
		sess := wal.Session{
			ID:          cfg.NewID(),
			RulesDigest: cfg.RulesDigest,
			Revision:    cfg.Revision,
			Modified:    cfg.Modified,
		}
		if g := a.terrain; g != nil {
			sess.TerrainCols, sess.TerrainRows = g.Cols, g.Rows
			sess.TerrainCellW, sess.TerrainCellH = g.CellW, g.CellH
			sess.Terrain = encodeTerrain(g)
		}
		return wal.Open(cfg.Dir, sess, wal.LogOptions{})
	})
	if err != nil {
		// Telemetry is never worth a game.
		slog.Error("cannot open the telemetry log; streaming off for this game", "error", err)
		return
	}
	if sink == nil {
		slog.Warn("telemetry already streaming for another connection; not streaming this one",
			"player", a.Player)
		return
	}
	log, ok := sink.(*wal.Log)
	if !ok {
		return
	}
	a.WAL = log
	slog.Info("streaming telemetry", "dir", cfg.Dir, "player", a.Player,
		"digest", cfg.RulesDigest, "modified", cfg.Modified)
}

func New(conn *ipc.Connection, engine *rules.Engine, strategist *Strategist, store *store.Store, ctx context.Context) *Agent {
	return &Agent{Conn: conn, Engine: engine, Strategist: strategist, Store: store, ctx: ctx}
}

// HandleHello completes the handshake so the mod knows the bridge is ready.
func (a *Agent) HandleHello(env ipc.Envelope) (*ipc.Envelope, error) {
	var hello ipc.HelloMessage
	if err := json.Unmarshal(env.Data, &hello); err != nil {
		return nil, fmt.Errorf("unmarshal hello: %w", err)
	}

	a.Player = hello.Player
	a.Faction = hello.Faction
	opponentSummary := make([]string, 0, len(hello.Opponents))
	for _, o := range hello.Opponents {
		opponentSummary = append(opponentSummary, o.Player+":"+o.Faction)
	}
	slog.Info("player identified",
		"player", a.Player,
		"faction", a.Faction,
		"opponents", opponentSummary)
	a.lastState.Store(time.Now().UnixNano())
	go a.watchForStall(stallAfter/3, stallAfter)

	if hello.Terrain != nil {
		grid := &model.TerrainGrid{
			Cols:  hello.Terrain.Cols,
			Rows:  hello.Terrain.Rows,
			CellW: hello.Terrain.CellW,
			CellH: hello.Terrain.CellH,
			Grid:  make([]model.TerrainType, len(hello.Terrain.Grid)),
		}
		for i, v := range hello.Terrain.Grid {
			grid.Grid[i] = model.TerrainType(v)
		}
		a.Engine.SetTerrain(grid)
		a.terrain = grid
	} else {
		slog.Warn("no terrain data in hello — terrain awareness disabled")
	}

	// After the terrain, so the session record carries it: it is static for the
	// whole game and belongs written down once rather than sampled.
	a.startTelemetry()

	if a.Strategist != nil {
		a.Strategist.SetFaction(hello.Faction)
		a.Strategist.SetOpponents(hello.Opponents)
		go a.Strategist.Start(a.ctx)
	}

	ack, err := ipc.NewEnvelope(ipc.TypeAck, ipc.AckMessage{Status: "ok"})
	if err != nil {
		return nil, err
	}
	return &ack, nil
}

// HandleGameEnd records the game outcome, then resets all accumulated state
// so the sidecar is ready for the next game without restarting the process.
func (a *Agent) HandleGameEnd(env ipc.Envelope) (*ipc.Envelope, error) {
	var msg ipc.GameEndMessage
	if err := json.Unmarshal(env.Data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal game_end: %w", err)
	}

	won := msg.Winner == a.Player
	slog.Info("game ended", "player", a.Player, "winner", msg.Winner, "won", won)

	// Before the review, not after: the retrospective inserts the archive row,
	// which names the export file, so the file must already exist. Logged rather
	// than returned — a failure here must not stop the game ending.
	exportPath, err := a.Engine.Exporter().Flush()
	if err != nil {
		slog.Error("failed to export rule evaluations", "error", err)
	}

	if a.Strategist != nil && strings.Contains(strings.ToLower(a.Player), "vimy") {
		a.Strategist.RecordGame(GameResult{
			Player:  a.Player,
			Faction: a.Faction,
			Won:     won,
		})

		// Under the lock and before Reset wipes it: the review runs async and
		// inserts the game record only once it finishes.
		snap := a.Strategist.snapshotForReview(won, exportPath)
		// game_id does not exist until ArchiveGame returns it, so sealing the
		// log is the retrospective's job. Without this the session ships with
		// game_id 0 and cannot be joined to anything.
		if snap != nil && a.WAL != nil {
			snap.onArchived = a.WAL.Finish
		}
		a.Strategist.runRetrospective(a.ctx, snap)
		if snap == nil && a.WAL != nil {
			// No retrospective to seal it. Still seal, or the open segment
			// never ships.
			if err := a.WAL.Finish(0); err != nil {
				slog.Error("sealing the telemetry log failed", "error", err)
			}
		}

		a.Strategist.Reset()

		// With no retrospective to run, still persist a win/loss row so the
		// dashboard counters stay correct.
		if snap == nil && a.Store != nil {
			_ = a.Store.RecordGame(store.GameRecord{Faction: a.Faction, Won: won})
		}
	}

	// Release the sink whatever happened above, or the next game finds it held
	// by a log that is already sealed and streams nothing.
	if a.WAL != nil {
		a.Engine.DetachEvents()
		a.WAL = nil
	}

	a.Engine.Reset()

	ack, err := ipc.NewEnvelope(ipc.TypeAck, ipc.AckMessage{Status: "ok"})
	if err != nil {
		return nil, err
	}
	return &ack, nil
}

func (a *Agent) HandleGameState(env ipc.Envelope) (*ipc.Envelope, error) {
	var gs model.GameState
	if err := json.Unmarshal(env.Data, &gs); err != nil {
		return nil, fmt.Errorf("unmarshal GameState: %w", err)
	}

	a.sampleUnits(gs)
	a.sampleThreat(gs)

	unitTypes := make(map[string]int)
	for _, u := range gs.Units {
		unitTypes[u.Type]++
	}
	buildingTypes := make(map[string]int)
	for _, b := range gs.Buildings {
		buildingTypes[b.Type]++
	}

	slog.Debug("game state received",
		"player", gs.Player.Name,
		"tick", gs.Tick,
		"cash", gs.Player.Cash,
		"resources", gs.Player.Resources,
		"power", fmt.Sprintf("%d/%d (%s)", gs.Player.PowerDrained, gs.Player.PowerProvided, gs.Player.PowerState),
		"buildings", buildingTypes,
		"units", unitTypes,
		"enemies", len(gs.Enemies),
		"queues", len(gs.ProductionQueues),
	)

	a.lastState.Store(time.Now().UnixNano())

	if err := a.Engine.Evaluate(gs, a.Faction, a.Conn); err != nil {
		slog.Error("rule engine error", "error", err)
	}

	if a.Strategist != nil {
		a.Strategist.UpdateState(gs)
	}

	ack, err := ipc.NewEnvelope(ipc.TypeAck, ipc.AckMessage{Status: "ok"})
	if err != nil {
		return nil, err
	}
	return &ack, nil
}

// stallAfter is how long a handshaken connection may go without a game state
// before the watchdog says so.
//
// Generous: a paused game or a slow frame must not cry wolf. The failure this
// exists for lasted eleven minutes.
const stallAfter = 30 * time.Second

// watchForStall complains when the handshake succeeded and then no game state
// was ever processed.
//
// Game 89 (2026-09-08): the previous match ended, OpenRA reconnected, `player
// identified` and `terrain grid set` both arrived, and then nothing — no
// doctrine, no rule set, no diagnostics — for eleven minutes, while the process
// sat alive at 1.8% CPU with the socket open and the dashboard still serving
// the previous game's numbers. Silence was the whole failure mode, so it read
// as the AI playing badly, and two people spent twenty minutes analysing a
// game that was not running (vimy-wma).
//
// This does not fix the stall. It makes the stall announce itself.
func (a *Agent) watchForStall(poll, after time.Duration) {
	t := time.NewTicker(poll)
	defer t.Stop()
	warned := false
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			last := a.lastState.Load()
			if last == 0 {
				continue
			}
			idle := time.Since(time.Unix(0, last))
			if idle < after {
				warned = false
				continue
			}
			if !warned {
				slog.Error("no game state processed since the handshake — the sidecar is not driving this game",
					"player", a.Player, "idle", idle.Round(time.Second))
				warned = true
			}
		}
	}
}

// threatSampleTicks is how often the danger map is written down.
//
// Far coarser than the unit feed because threat only moves when intel does - a
// defence sighted, a base found, one of ours dying somewhere new - and those
// are rare. Two hundred ticks is about 500 samples across a long game, and the
// field is sparse, so the whole thing is a few tens of thousands of rows.
const threatSampleTicks = 200

// sampleThreat records the field BestApproachAxis scores corridors against.
//
// It is the difference between "the front door was genuinely the best way in"
// and "we had seen nothing, so every corridor scored zero and the detour
// switched itself off". Establishing which cost an evening and a new counter;
// drawn on the map it is a glance.
func (a *Agent) sampleThreat(gs model.GameState) {
	if a.WAL == nil || a.terrain == nil || gs.Tick%threatSampleTicks != 0 {
		return
	}
	a.Engine.LockMemory()
	f := rules.ThreatFieldFor(a.Engine.Memory, a.terrain, gs)
	a.Engine.UnlockMemory()
	if f == nil {
		return
	}
	// Sparse on purpose: most zones are empty, and an empty field is the
	// finding rather than a gap in the data.
	for row := 0; row < f.Rows; row++ {
		for col := 0; col < f.Cols; col++ {
			if v := f.At(col, row); v > 0 {
				a.WAL.WriteThreat(wal.Threat{Tick: gs.Tick, Col: col, Row: row, Value: v})
			}
		}
	}
}

// unitSampleTicks is how often the whole field is written down.
//
// State arrives every 10 ticks, so every other update. At roughly 60 actors
// that is ~300k rows for a 100000-tick game, which lands near a megabyte once
// ClickHouse sorts them by tick - lighter than the rule-evaluation stream
// already running, because a unit that has not moved compresses to nothing.
//
// Twenty ticks is also under a second of game time, so a replay built from it
// reads as motion rather than as teleporting.
const unitSampleTicks = 20

// sampleUnits writes where everything on the field was, ours and theirs.
//
// Every diagnosis this telemetry has supported was made from scalars - spread,
// members, a distance ratio - and several were read wrong: a squad "arriving"
// that was two stragglers, a sawtooth blamed on three different rules before
// the right one, an approach that looked like bad routing and was missing
// intel. Positions are what those scalars summarise, and they are cheap.
func (a *Agent) sampleUnits(gs model.GameState) {
	if a.WAL == nil {
		return
	}
	if gs.Tick%unitSampleTicks != 0 {
		return
	}
	for _, u := range gs.Units {
		a.WAL.WriteUnit(wal.Unit{
			Tick: gs.Tick, ID: u.ID, Type: u.Type, Side: "ours",
			X: u.X, Y: u.Y, HP: u.HP, Idle: u.Idle,
		})
	}
	for _, b := range gs.Buildings {
		a.WAL.WriteUnit(wal.Unit{
			Tick: gs.Tick, ID: b.ID, Type: b.Type, Side: "ours",
			X: b.X, Y: b.Y, HP: b.HP, Building: true,
		})
	}
	// Enemies carries their buildings too - the game state does not separate
	// them - so the type decides which it is, the same way the targeting rules
	// decide it.
	for _, e := range gs.Enemies {
		a.WAL.WriteUnit(wal.Unit{
			Tick: gs.Tick, ID: e.ID, Type: e.Type, Side: "enemy",
			X: e.X, Y: e.Y, HP: e.HP, Building: rules.IsKnownBuildingType(e.Type),
		})
	}

	// And what it BELIEVES is out there. Sight is fleeting - 223 visible-enemy
	// rows against 12071 of ours in game 175 - while targeting and the approach
	// router run off remembered intel. A map without this shows an empty enemy
	// half of the field during a battle.
	a.Engine.LockMemory()
	bases := rules.RememberedEnemyBases(a.Engine.Memory)
	structs := rules.RememberedEnemyStructures(a.Engine.Memory)
	remembered := make([]wal.Unit, 0, len(bases)+len(structs))
	for owner, b := range bases {
		remembered = append(remembered, wal.Unit{
			Tick: gs.Tick, ID: 0, Type: "base:" + owner, Side: "enemy",
			X: b.X, Y: b.Y, Building: true, Remembered: true,
		})
	}
	for id, d := range structs {
		remembered = append(remembered, wal.Unit{
			Tick: gs.Tick, ID: id, Type: d.Type, Side: "enemy",
			X: d.X, Y: d.Y, Building: true, Remembered: true,
		})
	}
	a.Engine.UnlockMemory()
	for _, u := range remembered {
		a.WAL.WriteUnit(u)
	}
}

// encodeTerrain flattens the grid to one character per zone, row-major.
//
// A string rather than an array because it is written once per session and read
// by a renderer: 1024 characters is smaller than 1024 JSON numbers, and it is
// legible in a raw row, which matters for a field that will be looked at by a
// human before it is looked at by code.
func encodeTerrain(g *model.TerrainGrid) string {
	out := make([]byte, 0, len(g.Grid))
	for _, t := range g.Grid {
		switch t {
		case model.Water:
			out = append(out, '~')
		case model.Cliff:
			out = append(out, '#')
		case model.Bridge:
			out = append(out, '=')
		default:
			out = append(out, '.')
		}
	}
	return string(out)
}
