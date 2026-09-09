package agent

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
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
