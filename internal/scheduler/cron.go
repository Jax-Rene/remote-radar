package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"remote-radar/internal/fetcher"
	"remote-radar/internal/model"
	"remote-radar/internal/processor"
	"remote-radar/internal/storage"

	"golang.org/x/sync/errgroup"
)

// Config 用于调度配置。
type Config struct {
	Interval           string `yaml:"interval" json:"interval"`
	Timeout            string `yaml:"timeout" json:"timeout"`
	ProcessorBatchSize int    `yaml:"processor_batch_size" json:"processor_batch_size"`
}

// Store 抽象存储接口，便于测试替换。
type Store interface {
	UpsertJobs(ctx context.Context, jobs []model.Job) (storage.UpsertResult, error)
	UpsertRawJobs(ctx context.Context, jobs []model.RawJob) (storage.RawUpsertResult, error)
	ListRawJobs(ctx context.Context, query storage.RawJobQuery) ([]model.RawJob, error)
	UpdateRawJobStatus(ctx context.Context, id uint, update storage.RawJobStatusUpdate) error
}

// Notifier 用于发送新增职位通知。
type Notifier interface {
	Notify(ctx context.Context, jobs []model.Job) error
}

// Scheduler 负责周期性抓取并写入存储。
type Scheduler struct {
	fetcher   fetcher.JobFetcher
	store     Store
	processor processor.JobProcessor
	notif     Notifier
	interval  time.Duration
	timeout   time.Duration
	batchSize int
	running   atomic.Bool
	newTicker func(time.Duration) ticker
	now       func() time.Time
}

type ticker interface {
	C() <-chan time.Time
	Stop()
}

// NewScheduler 创建 Scheduler，解析配置的间隔与超时。
func NewScheduler(f fetcher.JobFetcher, s Store, proc processor.JobProcessor, n Notifier, cfg Config) *Scheduler {
	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil || interval <= 0 {
		interval = 2 * time.Hour
	}
	timeout := 30 * time.Second
	if cfg.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	batch := cfg.ProcessorBatchSize
	if batch <= 0 {
		batch = 20
	}

	return &Scheduler{
		fetcher:   f,
		store:     s,
		processor: proc,
		notif:     n,
		interval:  interval,
		timeout:   timeout,
		batchSize: batch,
		newTicker: defaultTicker,
		now:       time.Now,
	}
}

// Start 启动调度循环，直到上下文取消。
func (s *Scheduler) Start(ctx context.Context) error {
	if s.fetcher == nil || s.store == nil || s.processor == nil {
		return fmt.Errorf("scheduler missing dependencies")
	}

	g, ctx := errgroup.WithContext(ctx)

	tick := s.newTicker(s.interval)
	ch := tick.C()

	g.Go(func() error {
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ch:
				if _, err := s.runOnce(ctx); err != nil {
					return err
				}
			drain:
				for {
					select {
					case <-ch:
						continue
					default:
						break drain
					}
				}
			}
		}
	})

	return g.Wait()
}

// RunOnce 对外暴露单次抓取接口，便于手动刷新。
func (s *Scheduler) RunOnce(ctx context.Context) (int, error) {
	return s.runOnce(ctx)
}

func (s *Scheduler) runOnce(ctx context.Context) (int, error) {
	if s.running.Swap(true) {
		return 0, nil
	}
	defer s.running.Store(false)

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	jobs, err := s.fetcher.Fetch(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch jobs: %w", err)
	}

	rawJobs := make([]model.RawJob, 0, len(jobs))
	for _, job := range jobs {
		rawJobs = append(rawJobs, model.RawJob{
			Source:      job.Source,
			ExternalID:  job.ID,
			Title:       job.Title,
			Summary:     job.Summary,
			URL:         job.URL,
			Tags:        job.Tags,
			RawPayload:  job.RawAttributes,
			PublishedAt: job.PublishedAt,
		})
	}
	if _, err := s.store.UpsertRawJobs(ctx, rawJobs); err != nil {
		return 0, fmt.Errorf("upsert raw jobs: %w", err)
	}

	pending, err := s.store.ListRawJobs(ctx, storage.RawJobQuery{Status: model.RawJobStatusPending, Limit: s.batchSize})
	if err != nil {
		return 0, fmt.Errorf("list raw jobs: %w", err)
	}

	processed := make([]model.Job, 0, len(pending))
	for _, raw := range pending {
		res, err := s.processor.Process(ctx, raw)
		if err != nil {
			return 0, fmt.Errorf("process raw job %d: %w", raw.ID, err)
		}

		update := storage.RawJobStatusUpdate{Status: model.RawJobStatusRejected, Reason: res.Reason, Details: res.Trace}
		if res.Outcome == processor.ResultAccepted && res.Job != nil {
			processed = append(processed, *res.Job)
			update.Status = model.RawJobStatusProcessed
			update.Reason = ""
		}
		if err := s.store.UpdateRawJobStatus(ctx, raw.ID, update); err != nil {
			return 0, fmt.Errorf("update raw job status: %w", err)
		}
	}

	if len(processed) == 0 {
		return 0, nil
	}

	res, err := s.store.UpsertJobs(ctx, processed)
	if err != nil {
		return 0, fmt.Errorf("upsert jobs: %w", err)
	}

	if s.notif != nil && len(res.NewJobs) > 0 {
		if err := s.notif.Notify(ctx, res.NewJobs); err != nil {
			return res.Created, fmt.Errorf("notify: %w", err)
		}
	}

	return res.Created, nil
}

func defaultTicker(d time.Duration) ticker {
	t := time.NewTicker(d)
	return tickerWrapper{t}
}

type tickerWrapper struct {
	*time.Ticker
}

func (t tickerWrapper) C() <-chan time.Time { return t.Ticker.C }
func (t tickerWrapper) Stop()               { t.Ticker.Stop() }
