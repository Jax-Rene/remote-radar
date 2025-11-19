package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"remote-radar/internal/api"
	"remote-radar/internal/fetcher"
	"remote-radar/internal/model"
	"remote-radar/internal/notifier"
	"remote-radar/internal/scheduler"
	"remote-radar/internal/storage"

	"golang.org/x/sync/errgroup"
)

// AppConfig 应用配置。
type AppConfig struct {
	Fetcher  fetcher.Config       `yaml:"fetcher"`
	Email    notifier.EmailConfig `yaml:"email"`
	Notifier NotifierConfig       `yaml:"notifier"`
	Server   ServerConfig         `yaml:"server"`
	Database DatabaseConfig       `yaml:"database"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// NotifierConfig 控制通知方式。
type NotifierConfig struct {
	Driver string `yaml:"driver"`
}

const defaultShutdownTimeout = 5 * time.Second

type schedulerRunner interface {
	Start(context.Context) error
	RunOnce(context.Context) (int, error)
}

type serverRunner interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

type appDeps struct {
	store *storage.Store
	sched schedulerRunner
}

func main() {
	var runOnce bool
	flag.BoolVar(&runOnce, "once", false, "run crawler once and exit")
	flag.Parse()

	cfg, err := loadConfig()
	if err != nil {
		log.Printf("load config error: %v", err)
		return
	}

	if runOnce {
		created, err := runOnceManual(context.Background(), cfg, buildApp)
		if err != nil {
			log.Printf("run once error: %v", err)
			return
		}
		log.Printf("run once finished: created=%d", created)
		return
	}

	deps, cleanup, err := buildApp(cfg)
	if err != nil {
		log.Printf("init app error: %v", err)
		return
	}
	defer cleanup()

	handler := api.NewHandler(storeAdapter{deps.store}, schedulerAdapter{deps.sched})

	addr := cfg.Server.Addr
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{Addr: addr, Handler: handler}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("listening on %s", addr)
	if err := runServer(ctx, srv, deps.sched, defaultShutdownTimeout); err != nil {
		log.Printf("server stopped: %v", err)
	}
}

// runServer 启动 HTTP 服务与调度器，并在上下文取消时优雅关闭。
func runServer(ctx context.Context, srv serverRunner, sched schedulerRunner, shutdownTimeout time.Duration) error {
	if srv == nil {
		return fmt.Errorf("run server: %w", errors.New("http server is nil"))
	}
	if sched == nil {
		return fmt.Errorf("run server: %w", errors.New("scheduler is nil"))
	}
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := sched.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("start scheduler: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("listen and serve: %w", err)
		}
		return nil
	})

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("shutdown server: %w", err)
	}

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func runOnceManual(ctx context.Context, cfg AppConfig, builder func(AppConfig) (appDeps, func(), error)) (int, error) {
	deps, cleanup, err := builder(cfg)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return 0, fmt.Errorf("build app: %w", err)
	}
	return deps.sched.RunOnce(ctx)
}

func buildApp(cfg AppConfig) (appDeps, func(), error) {
	dbPath := cfg.Database.Path
	if dbPath == "" {
		dbPath = "jobs.db"
	}

	store, err := storage.NewStore(dbPath)
	if err != nil {
		return appDeps{}, nil, fmt.Errorf("init store: %w", err)
	}

	cleanup := func() {
		store.Close()
	}

	client := &http.Client{Timeout: 15 * time.Second}
	fetch := fetcher.NewEleduckFetcher("https://eleduck.com", cfg.Fetcher, client)
	notif := selectNotifier(cfg)
	sched := scheduler.NewScheduler(fetch, store, notif, scheduler.Config{Interval: cfg.Fetcher.Interval, Timeout: "30s"})

	return appDeps{store: store, sched: sched}, cleanup, nil
}

// selectNotifier 根据配置决定使用哪种通知方式。
func selectNotifier(cfg AppConfig) scheduler.Notifier {
	driver := strings.ToLower(strings.TrimSpace(cfg.Notifier.Driver))
	switch driver {
	case "log":
		return notifier.NewLogNotifier(nil)
	case "", "email":
		if cfg.Email.Host == "" || cfg.Email.From == "" || len(cfg.Email.To) == 0 {
			log.Printf("email config incomplete, fallback to log notifier")
			return notifier.NewLogNotifier(nil)
		}
		return notifier.NewEmailNotifier(cfg.Email, nil)
	default:
		log.Printf("unknown notifier driver %q, fallback to log notifier", driver)
		return notifier.NewLogNotifier(nil)
	}
}

func loadConfig() (AppConfig, error) {
	path := os.Getenv("CONFIG_FILE")
	if path == "" {
		path = "config.yaml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return AppConfig{}, err
	}
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, err
	}
	return cfg, nil
}

// 适配 API 所需接口。
type storeAdapter struct {
	store *storage.Store
}

func (s storeAdapter) ListJobs(_ *http.Request, limit, offset int) ([]model.Job, error) {
	return s.store.ListJobs(context.Background(), limit, offset)
}

func (s storeAdapter) CountJobs(_ *http.Request) (int64, error) {
	return s.store.CountJobs(context.Background())
}

type schedulerAdapter struct {
	sched schedulerRunner
}

func (s schedulerAdapter) RunOnce(_ *http.Request) (int, error) {
	return s.sched.RunOnce(context.Background())
}
