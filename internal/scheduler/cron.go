package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"remote-radar/internal/fetcher"
	"remote-radar/internal/model"
	"remote-radar/internal/storage"

	"golang.org/x/sync/errgroup"
)

// Config 用于调度配置。
type Config struct {
	Interval string `yaml:"interval" json:"interval"`
	Timeout  string `yaml:"timeout" json:"timeout"`
}

// Store 抽象存储接口，便于测试替换。
type Store interface {
	UpsertJobs(ctx context.Context, jobs []model.Job) (storage.UpsertResult, error)
}

// Notifier 用于发送新增职位通知。
type Notifier interface {
	Notify(ctx context.Context, jobs []model.Job) error
}

// Scheduler 负责周期性抓取并写入存储。
type Scheduler struct {
	fetcher   fetcher.JobFetcher
	store     Store
	notif     Notifier
	interval  time.Duration
	timeout   time.Duration
	running   atomic.Bool
	newTicker func(time.Duration) ticker
	now       func() time.Time
}

type ticker interface {
	C() <-chan time.Time
	Stop()
}

// NewScheduler 创建 Scheduler，解析配置的间隔与超时。
func NewScheduler(f fetcher.JobFetcher, s Store, n Notifier, cfg Config) *Scheduler {
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

	return &Scheduler{
		fetcher:   f,
		store:     s,
		notif:     n,
		interval:  interval,
		timeout:   timeout,
		newTicker: defaultTicker,
		now:       time.Now,
	}
}

// Start 启动调度循环，直到上下文取消。
func (s *Scheduler) Start(ctx context.Context) error {
	if s.fetcher == nil || s.store == nil {
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
		return 0, nil // 已在运行，跳过
	}
	defer s.running.Store(false)

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	jobs, err := s.fetcher.Fetch(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch jobs: %w", err)
	}

	res, err := s.store.UpsertJobs(ctx, jobs)
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
