package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestJobsShareWorkAndRetainResults(t *testing.T) {
	r := newJobRunner(1, 4)
	defer r.Close()
	var jobs jobSet[int]
	var calls atomic.Int32
	release := make(chan struct{})
	run := func(ctx context.Context) (int, bool, error) {
		calls.Add(1)
		select {
		case <-release:
			return 42, true, nil
		case <-ctx.Done():
			return 0, false, ctx.Err()
		}
	}
	key := analysisKey{Game: 1, Revision: "a"}
	first := jobs.start(r, key, time.Minute, run)
	second := jobs.start(r, key, time.Minute, run)
	if first != second {
		t.Fatal("duplicated in-flight work")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := first.wait(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	if result, err := second.wait(t.Context()); err != nil || result != 42 {
		t.Fatalf("%d %v", result, err)
	}
	if jobs.start(r, key, time.Minute, run) != first || calls.Load() != 1 {
		t.Fatal("repeated completed work")
	}
	changed := jobs.start(r, analysisKey{Game: 1, Revision: "b"}, time.Minute, run)
	if changed == first {
		t.Fatal("reused old input revision")
	}
	if _, err := changed.wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestJobsRetryFailedAndUnsettledResults(t *testing.T) {
	for _, failed := range []bool{true, false} {
		t.Run(map[bool]string{true: "failure", false: "unsettled"}[failed], func(t *testing.T) {
			r := newJobRunner(1, 4)
			defer r.Close()
			var jobs jobSet[int]
			key := analysisKey{Game: 1}
			first := jobs.start(r, key, time.Minute, func(context.Context) (int, bool, error) {
				if failed {
					return 0, false, errors.New("failed")
				}
				return 1, false, nil
			})
			<-first.done
			if jobs.get(1) != first {
				t.Fatal("polling lost the finished result")
			}
			second := jobs.start(r, key, time.Minute, func(context.Context) (int, bool, error) { return 2, true, nil })
			if second == first {
				t.Fatal("retry was blocked")
			}
			if value, err := second.wait(t.Context()); err != nil || value != 2 {
				t.Fatalf("%d %v", value, err)
			}
		})
	}
}

func TestRunnerBoundsQueueAndCancelsShutdown(t *testing.T) {
	r := newJobRunner(1, 2)
	var jobs jobSet[int]
	started := make(chan struct{})
	blocked := func(ctx context.Context) (int, bool, error) { close(started); <-ctx.Done(); return 0, false, ctx.Err() }
	first := jobs.start(r, analysisKey{Game: 1}, time.Minute, blocked)
	<-started
	second := jobs.start(r, analysisKey{Game: 2}, time.Minute, func(ctx context.Context) (int, bool, error) {
		t.Error("queued work started after shutdown")
		return 0, false, nil
	})
	rejected := jobs.start(r, analysisKey{Game: 3}, time.Minute, blocked)
	if _, err := rejected.wait(t.Context()); err == nil {
		t.Fatal("queue was not bounded")
	}
	r.Close()
	for _, j := range []*job[int]{first, second} {
		if _, err := j.wait(t.Context()); !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown error: %v", err)
		}
	}
	after := jobs.start(r, analysisKey{Game: 4}, time.Minute, blocked)
	if _, err := after.wait(t.Context()); !errors.Is(err, context.Canceled) {
		t.Fatal("work accepted after shutdown")
	}
}
