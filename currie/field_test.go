package main

import (
	"bytes"
	"strings"
	"testing"
)

// A game of any length should take about the same time to watch, which is what
// makes two replays comparable by eye. One sample per frame does not: game 174
// is 2,820 samples, and at a watchable frame rate that is a six-minute replay.
func TestPlayStepSizesTheReplayNotTheSample(t *testing.T) {
	// A long game steps several samples at a time.
	step := playStep(20, 56400)
	if step <= fieldScrubStride {
		t.Fatalf("step %d: a 56k-tick game must advance faster than one sample", step)
	}
	if frames := 56400 / step; frames < 200 || frames > 400 {
		t.Errorf("%d frames for a long game, want roughly %d", frames, playFrames)
	}
	// Whatever the arithmetic says, a step finer than the sampling stride
	// shows the same field twice.
	if step%fieldScrubStride != 0 {
		t.Errorf("step %d is not a whole number of samples", step)
	}

	// A short game never steps finer than the samples it has.
	if got := playStep(20, 600); got != fieldScrubStride {
		t.Errorf("short game step = %d, want %d", got, fieldScrubStride)
	}
}

// Playback walks the past; live follows the present. A frame doing both would
// jump to the newest sample mid-replay.
func TestClockPlaybackDoesNotGoLive(t *testing.T) {
	v := &fieldView{Tick: 4000}
	v.clock(20, 56400, true)
	if v.Live {
		t.Error("a playing frame must not also be live")
	}
	if !v.Playing {
		t.Error("mid-timeline should keep playing")
	}
	if v.NextTick <= v.Tick {
		t.Errorf("next %d does not advance past %d", v.NextTick, v.Tick)
	}
}

// The last frame stops rather than asking forever for a tick it already shows,
// and the control then offers the only move left: back to the beginning.
func TestClockStopsAtTheEndAndOffersReplay(t *testing.T) {
	v := &fieldView{Tick: 56400}
	v.clock(20, 56400, true)
	if v.Playing {
		t.Error("the last frame must not schedule another")
	}
	if v.NextTick != 56400 {
		t.Errorf("next = %d, want the end", v.NextTick)
	}
	if v.PlayFrom != 20 {
		t.Errorf("PlayFrom = %d, want the start so the control replays", v.PlayFrom)
	}
}

// Landing one step short of the end must reach the end exactly, not overshoot
// into a tick with no sample behind it.
func TestClockClampsTheFinalStep(t *testing.T) {
	hi := 56400
	v := &fieldView{Tick: hi - 10}
	v.clock(20, hi, true)
	if v.NextTick != hi {
		t.Errorf("next = %d, want %d", v.NextTick, hi)
	}
	if !v.Playing {
		t.Error("one step short of the end is still playing")
	}
}

// Pressing play from the live view starts at the beginning: there is nothing
// forward of the newest sample to play.
func TestClockPlayFromLiveStartsAtTheBeginning(t *testing.T) {
	v := &fieldView{}
	v.clock(20, 56400, false)
	if !v.Live || v.Tick != 56400 {
		t.Fatalf("live = %v, tick = %d", v.Live, v.Tick)
	}
	if v.PlayFrom != 20 {
		t.Errorf("PlayFrom = %d, want the start", v.PlayFrom)
	}
}

// A paused frame in the middle resumes from where it is, not from the start.
func TestClockPausedResumesInPlace(t *testing.T) {
	v := &fieldView{Tick: 4000}
	v.clock(20, 56400, false)
	if v.Playing || v.Live {
		t.Error("a pinned tick is neither playing nor live")
	}
	if v.PlayFrom != 4000 {
		t.Errorf("PlayFrom = %d, want 4000", v.PlayFrom)
	}
}

// A playing frame carries its own next request; a paused one must not, or it
// would keep advancing after the pause.
func TestFieldFrameSchedulesOnlyWhilePlaying(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	render := func(v *fieldView) string {
		var buf bytes.Buffer
		if err := tmpl.ExecuteTemplate(&buf, "fieldframe", v); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	playing := &fieldView{Session: "s1", Tick: 4000, Size: fieldSize}
	playing.clock(20, 56400, true)
	out := render(playing)
	if !strings.Contains(out, "hx-trigger=\"load delay:"+playDelay) {
		t.Error("a playing frame must schedule the next one")
	}
	if !strings.Contains(out, "pause") {
		t.Error("a playing frame must offer pause")
	}

	paused := &fieldView{Session: "s1", Tick: 4000, Size: fieldSize}
	paused.clock(20, 56400, false)
	out = render(paused)
	if strings.Contains(out, "load delay:") {
		t.Error("a paused frame must not schedule another")
	}
	if !strings.Contains(out, "play=1") {
		t.Error("a paused frame must offer play")
	}
}
