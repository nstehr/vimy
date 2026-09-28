package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// jobRunner owns the server lifetime and bounds both running and queued work.
// A disconnected browser stops waiting, but does not cancel a shared paid job.
type jobRunner struct {
	ctx     context.Context
	cancel  context.CancelFunc
	workers chan struct{}
	slots   chan struct{}
	mu      sync.Mutex
	closed  bool
	wg      sync.WaitGroup
}

func newJobRunner(workers, capacity int) *jobRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &jobRunner{ctx: ctx, cancel: cancel, workers: make(chan struct{}, workers), slots: make(chan struct{}, capacity)}
}

func (r *jobRunner) submit(timeout time.Duration, run func(context.Context), reject func(error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		reject(context.Canceled)
		return
	}
	select {
	case r.slots <- struct{}{}:
	default:
		reject(fmt.Errorf("analysis queue is full; retry later"))
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer func() { <-r.slots }()
		ctx, cancel := context.WithTimeout(r.ctx, timeout)
		defer cancel()
		select {
		case r.workers <- struct{}{}:
			defer func() { <-r.workers }()
		case <-ctx.Done():
			reject(ctx.Err())
			return
		}
		if err := ctx.Err(); err != nil {
			reject(err)
			return
		}
		run(ctx)
	}()
}
func (r *jobRunner) Close() { r.mu.Lock(); r.closed = true; r.cancel(); r.mu.Unlock(); r.wg.Wait() }

type analysisKey struct {
	Game     int64
	Revision string
}

type job[T any] struct {
	done     chan struct{}
	value    T
	err      error
	reusable bool
	created  time.Time
}

// Result fields are immutable after done closes. Failed or non-final results
// stay available to polling requests and are retried only on a new Start.
type jobSet[T any] struct {
	mu     sync.Mutex
	jobs   map[analysisKey]*job[T]
	latest map[int64]analysisKey
}

func (s *jobSet[T]) start(r *jobRunner, key analysisKey, timeout time.Duration, run func(context.Context) (T, bool, error)) *job[T] {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jobs == nil {
		s.jobs = map[analysisKey]*job[T]{}
		s.latest = map[int64]analysisKey{}
	}
	s.latest[key.Game] = key
	if prior := s.jobs[key]; prior != nil {
		select {
		case <-prior.done:
			if prior.err == nil && prior.reusable {
				return prior
			}
		default:
			return prior
		}
	}
	// Completed results are bounded independently of the execution queue.
	if len(s.jobs) >= 64 {
		var oldest analysisKey
		var at time.Time
		for k, j := range s.jobs {
			select {
			case <-j.done:
				if at.IsZero() || j.created.Before(at) {
					oldest, at = k, j.created
				}
			default:
			}
		}
		if !at.IsZero() {
			delete(s.jobs, oldest)
			if s.latest[oldest.Game] == oldest {
				delete(s.latest, oldest.Game)
			}
		}
	}
	j := &job[T]{done: make(chan struct{}), created: time.Now()}
	s.jobs[key] = j
	s.latest[key.Game] = key
	r.submit(timeout, func(ctx context.Context) { j.value, j.reusable, j.err = run(ctx); close(j.done) }, func(err error) { j.err = err; close(j.done) })
	return j
}
func (s *jobSet[T]) get(game int64) *job[T] {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[s.latest[game]]
}
func (j *job[T]) wait(ctx context.Context) (T, error) {
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case <-j.done:
		return j.value, j.err
	}
}
