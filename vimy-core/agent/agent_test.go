package agent

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nstehr/vimy/vimy-core/model"
	"github.com/nstehr/vimy/vimy-core/rules"
	"github.com/nstehr/vimy/vimy-core/wal"
)

// The stall in game 89 announced itself as silence: handshake fine, then no
// game state for eleven minutes while the process sat alive with the socket
// open. Silence must not be the failure mode.
func TestWatchdogFiresWhenNoStateArrives(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))
	defer slog.SetDefault(prev)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &Agent{Player: "bot", ctx: ctx}
	// Handshaken a long time ago, never fed a state.
	a.lastState.Store(time.Now().Add(-time.Minute).UnixNano())
	go a.watchForStall(5*time.Millisecond, 50*time.Millisecond)

	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(buf.String(), "not driving this game") {
			return
		}
		select {
		case <-deadline:
			t.Fatal("watchdog stayed silent while no game state arrived")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// A game that is being played must not trip it.
func TestWatchdogQuietWhileStateFlows(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))
	defer slog.SetDefault(prev)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &Agent{Player: "bot", ctx: ctx}
	go a.watchForStall(5*time.Millisecond, time.Second)
	for i := 0; i < 20; i++ {
		a.lastState.Store(time.Now().UnixNano())
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Contains(buf.String(), "not driving") {
		t.Errorf("watchdog cried wolf on a live game: %s", buf.String())
	}
}

// Unit and threat sampling must not depend on the tick PHASE.
//
// `gs.Tick % 20 == 0` was the gate. State arrives every ten ticks but its phase
// is whatever the tick was when the sidecar connected, and nothing aligns it to
// zero: game 196's states landed on ticks congruent to 0 and 10 mod 20 and
// sampled normally, and the very next game's landed on 1 and 11, so not one unit
// or threat row was written for the whole game while evals streamed as usual.
func TestSamplingSurvivesAnOddTickPhase(t *testing.T) {
	dir := t.TempDir()
	log, err := wal.Open(dir, wal.Session{ID: "phase-test", StartedAt: time.Now()},
		wal.LogOptions{RowsPerSegment: 1})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := rules.NewEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &Agent{WAL: log, Engine: eng}
	a.lastUnitSample = -unitSampleTicks

	// Ticks congruent to 1 mod 20: every one misses a `% 20` gate.
	for tick := 901; tick <= 1101; tick += 10 {
		a.sampleUnits(model.GameState{
			Tick:  tick,
			Units: []model.Unit{{ID: 1, Type: "e1", X: 5, Y: 5, HP: 50, MaxHP: 50}},
		})
	}
	if err := log.Finish(0); err != nil {
		t.Fatal(err)
	}

	names, err := os.ReadDir(filepath.Join(dir, "phase-test"))
	if err != nil {
		t.Fatal(err)
	}
	units := 0
	for _, n := range names {
		if strings.HasPrefix(n.Name(), wal.UnitsPrefix) {
			units++
		}
	}
	if units == 0 {
		var got []string
		for _, n := range names {
			got = append(got, n.Name())
		}
		t.Errorf("no unit segments written across 200 ticks of odd-phase state; dir holds %v", got)
	}
}
