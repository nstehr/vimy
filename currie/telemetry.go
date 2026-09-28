package main

import (
	"context"

	"github.com/nstehr/vimy/currie/ch"
)

// telemetryReader owns field query semantics shared by the UI and model tools.
// An error is never represented as an empty observation.
type telemetryReader struct{ client *ch.Client }

type unitExtent struct {
	Lo    int `json:"lo"`
	Hi    int `json:"hi"`
	Count int `json:"n"`
}

func (r telemetryReader) UnitExtent(ctx context.Context, session string) (unitExtent, error) {
	return ch.One[unitExtent](ctx, r.client, `SELECT min(tick) AS lo, max(tick) AS hi, count() AS n FROM stream_units WHERE session_id = {session:String}`, map[string]any{"session": session})
}
func (r telemetryReader) UnitsAt(ctx context.Context, session string, tick int) ([]unitRow, error) {
	return ch.Query[unitRow](ctx, r.client, `SELECT tick, unit_id, type, side, x, y, hp, idle, is_building, remembered
 FROM stream_units
 WHERE session_id = {session:String}
 AND tick = (SELECT max(tick) FROM stream_units WHERE session_id = {session:String} AND tick <= {tick:UInt32})`, map[string]any{"session": session, "tick": tick})
}

type threatSnapshot struct {
	// Recorded means at least one positive threat cell exists somewhere in the
	// session. Sparse legacy feeds cannot prove whether a wholly empty feed ran.
	Recorded bool
	Cells    []threatCell
	Peak     float64
}

func (r telemetryReader) ThreatAt(ctx context.Context, session string, tick int) (threatSnapshot, error) {
	var result threatSnapshot
	count, err := ch.One[struct {
		N int `json:"n"`
	}](ctx, r.client, `SELECT count() AS n FROM stream_threat WHERE session_id = {session:String}`, map[string]any{"session": session})
	if err != nil {
		return result, err
	}
	result.Recorded = count.N > 0
	if !result.Recorded {
		return result, nil
	}
	result.Cells, err = ch.Query[threatCell](ctx, r.client, `SELECT col, row, value FROM stream_threat
 WHERE session_id = {session:String}
 AND tick = (SELECT max(tick) FROM stream_threat WHERE session_id = {session:String} AND tick <= {tick:UInt32})`, map[string]any{"session": session, "tick": tick})
	if err != nil {
		return threatSnapshot{}, err
	}
	for _, cell := range result.Cells {
		result.Peak = max(result.Peak, cell.Value)
	}
	return result, nil
}
