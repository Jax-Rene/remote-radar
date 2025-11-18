package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"remote-radar/internal/model"
	"remote-radar/internal/storage"
)

func TestSchedulerRunOnce(t *testing.T) {
	t.Parallel()

	f := &stubFetcher{
		jobs: []model.Job{{ID: "1"}, {ID: "2"}},
	}
	s := &stubStore{}

	sched := NewScheduler(f, s, nil, Config{Interval: "1h", Timeout: "5s"})

	created, err := sched.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if created != 2 {
		t.Fatalf("expected 2 created jobs, got %d", created)
	}
	if f.calls.Load() != 1 {
		t.Fatalf("expected fetcher called once, got %d", f.calls.Load())
	}
	if s.calls.Load() != 1 {
		t.Fatalf("expected store called once, got %d", s.calls.Load())
	}
}

func TestSchedulerNoOverlap(t *testing.T) {
	t.Parallel()

	tickCh := make(chan time.Time, 4)
	st := &stubTicker{ch: tickCh}

	f := &stubFetcher{
		jobs:  []model.Job{{ID: "1"}},
		block: make(chan struct{}),
	}
	s := &stubStore{}

	sched := NewScheduler(f, s, nil, Config{Interval: "100ms", Timeout: "5s"})
	sched.newTicker = func(d time.Duration) ticker { return st }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = sched.Start(ctx)
	}()

	// Trigger first tick; fetcher blocks until we release.
	tickCh <- time.Now()
	time.Sleep(20 * time.Millisecond)

	// Trigger second tick while first run is still in progress.
	tickCh <- time.Now()

	// Allow first run to finish.
	close(f.block)

	// Wait for scheduler to process and then stop.
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if f.calls.Load() != 1 {
		t.Fatalf("expected fetcher called once due to overlap prevention, got %d", f.calls.Load())
	}
	if s.calls.Load() != 1 {
		t.Fatalf("expected store called once, got %d", s.calls.Load())
	}
}

func TestSchedulerNotifiesNewJobs(t *testing.T) {
	t.Parallel()

	f := &stubFetcher{
		jobs: []model.Job{{ID: "n1"}},
	}
	s := &stubStore{}
	n := &stubNotifier{}

	sched := NewScheduler(f, s, n, Config{Interval: "1h", Timeout: "5s"})

	created, err := sched.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected 1 created, got %d", created)
	}
	if n.calls.Load() != 1 {
		t.Fatalf("expected notifier called once, got %d", n.calls.Load())
	}
}

// --- stubs ---

type stubFetcher struct {
	jobs  []model.Job
	err   error
	calls atomic.Int32
	block chan struct{}
}

func (s *stubFetcher) Fetch(ctx context.Context) ([]model.Job, error) {
	s.calls.Add(1)
	if s.block != nil {
		<-s.block
	}
	return s.jobs, s.err
}

type stubStore struct {
	calls atomic.Int32
	mu    sync.Mutex
	saved []model.Job
	err   error
}

func (s *stubStore) UpsertJobs(ctx context.Context, jobs []model.Job) (storage.UpsertResult, error) {
	s.calls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, jobs...)
	return storage.UpsertResult{Created: len(jobs), NewJobs: jobs}, s.err
}

type stubTicker struct {
	ch chan time.Time
}

func (s *stubTicker) C() <-chan time.Time { return s.ch }
func (s *stubTicker) Stop()               {}

type stubNotifier struct {
	calls atomic.Int32
}

func (n *stubNotifier) Notify(ctx context.Context, jobs []model.Job) error {
	n.calls.Add(1)
	return ctx.Err()
}
