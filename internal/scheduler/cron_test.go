package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"remote-radar/internal/model"
	"remote-radar/internal/processor"
	"remote-radar/internal/storage"
)

func TestSchedulerRunOnceProcessesRawJobs(t *testing.T) {
	t.Parallel()

	f := &stubFetcher{jobs: []model.Job{{ID: "raw1", Title: "Job1"}}}

	store := &stubStore{}
	newRaw := model.RawJob{ID: 1, ExternalID: "raw1", Source: "eleduck", Title: "Job1"}
	backlog := model.RawJob{ID: 2, ExternalID: "raw-legacy", Source: "eleduck", Title: "Legacy"}
	store.rawUpsertResult = storage.RawUpsertResult{Created: 1, NewJobs: []model.RawJob{newRaw}}
	store.pending = []model.RawJob{newRaw, backlog}
	store.jobResult = storage.UpsertResult{Created: 2, NewJobs: []model.Job{{ID: "raw1"}, {ID: "raw-legacy"}}}

	proc := &stubProcessor{
		results: map[string]processor.Result{
			"raw1": {
				Outcome: processor.ResultAccepted,
				Job:     &model.Job{ID: "raw1", Title: "Job1"},
			},
			"raw-legacy": {
				Outcome: processor.ResultAccepted,
				Job:     &model.Job{ID: "raw-legacy", Title: "Legacy"},
			},
		},
	}

	n := &stubNotifier{}

	sched := NewScheduler(f, store, proc, n, Config{Interval: "1h", Timeout: "5s", ProcessorBatchSize: 5})

	created, err := sched.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if created != 2 {
		t.Fatalf("expected 2 created jobs, got %d", created)
	}
	if store.upsertCalls.Load() != 1 {
		t.Fatalf("expected final job upsert called once, got %d", store.upsertCalls.Load())
	}
	if len(store.statusUpdates) != 2 {
		t.Fatalf("expected two status updates, got %d", len(store.statusUpdates))
	}
	if n.calls.Load() != 1 {
		t.Fatalf("expected notifier called once, got %d", n.calls.Load())
	}
}

func TestSchedulerNoOverlap(t *testing.T) {
	t.Parallel()

	tickCh := make(chan time.Time, 4)
	st := &stubTicker{ch: tickCh}

	f := &stubFetcher{
		jobs:  []model.Job{{ID: "x"}},
		block: make(chan struct{}),
	}
	store := &stubStore{jobResult: storage.UpsertResult{Created: 1, NewJobs: []model.Job{{ID: "x"}}}, pending: []model.RawJob{{ID: 1, ExternalID: "x", Source: "eleduck"}}}
	proc := &stubProcessor{
		results: map[string]processor.Result{
			"x": {Outcome: processor.ResultAccepted, Job: &model.Job{ID: "x"}},
		},
	}

	sched := NewScheduler(f, store, proc, nil, Config{Interval: "100ms", Timeout: "5s", ProcessorBatchSize: 1})
	sched.newTicker = func(time.Duration) ticker { return st }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = sched.Start(ctx)
	}()

	tickCh <- time.Now()
	time.Sleep(20 * time.Millisecond)

	tickCh <- time.Now()

	close(f.block)

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if f.calls.Load() != 1 {
		t.Fatalf("expected fetcher called once due to overlap prevention, got %d", f.calls.Load())
	}
	if store.upsertCalls.Load() != 1 {
		t.Fatalf("expected store upsert called once, got %d", store.upsertCalls.Load())
	}
}

func TestSchedulerNotifiesOnlyWhenNewJobs(t *testing.T) {
	t.Parallel()

	f := &stubFetcher{jobs: []model.Job{{ID: "notify"}}}
	store := &stubStore{}
	store.rawUpsertResult = storage.RawUpsertResult{Created: 1, NewJobs: []model.RawJob{{ID: 1, ExternalID: "notify", Source: "eleduck", Title: "Notify"}}}
	store.pending = []model.RawJob{{ID: 1, ExternalID: "notify", Source: "eleduck", Title: "Notify"}}
	store.jobResult = storage.UpsertResult{Created: 1, NewJobs: []model.Job{{ID: "notify"}}}

	proc := &stubProcessor{
		results: map[string]processor.Result{
			"notify": {Outcome: processor.ResultAccepted, Job: &model.Job{ID: "notify"}},
		},
	}
	n := &stubNotifier{}

	sched := NewScheduler(f, store, proc, n, Config{Interval: "1h", Timeout: "5s", ProcessorBatchSize: 2})

	created, err := sched.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected 1 created job, got %d", created)
	}
	if n.calls.Load() != 1 {
		t.Fatalf("expected notifier called once when new jobs exist, got %d", n.calls.Load())
	}

	// No new jobs second call -> notifier should not fire.
	store.jobResult = storage.UpsertResult{Created: 0, NewJobs: nil}
	created, err = sched.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce second call error: %v", err)
	}
	if created != 0 {
		t.Fatalf("expected 0 created on second run, got %d", created)
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
	rawUpsertResult storage.RawUpsertResult
	jobResult       storage.UpsertResult
	pending         []model.RawJob
	statusUpdates   []statusRecord
	upsertCalls     atomic.Int32
	mu              sync.Mutex
}

type statusRecord struct {
	id     uint
	update storage.RawJobStatusUpdate
}

func (s *stubStore) UpsertRawJobs(ctx context.Context, jobs []model.RawJob) (storage.RawUpsertResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rawUpsertResult, nil
}

func (s *stubStore) ListRawJobs(ctx context.Context, q storage.RawJobQuery) ([]model.RawJob, error) {
	return append([]model.RawJob(nil), s.pending...), nil
}

func (s *stubStore) UpdateRawJobStatus(ctx context.Context, id uint, update storage.RawJobStatusUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusUpdates = append(s.statusUpdates, statusRecord{id: id, update: update})
	return nil
}

func (s *stubStore) UpsertJobs(ctx context.Context, jobs []model.Job) (storage.UpsertResult, error) {
	s.upsertCalls.Add(1)
	return s.jobResult, nil
}

type stubProcessor struct {
	mu      sync.Mutex
	results map[string]processor.Result
}

func (s *stubProcessor) Process(ctx context.Context, raw model.RawJob) (processor.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if res, ok := s.results[raw.ExternalID]; ok {
		return res, nil
	}
	return processor.Result{Outcome: processor.ResultRejected, Reason: "missing"}, nil
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
