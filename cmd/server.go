package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"remote-radar/internal/api"
	"remote-radar/internal/fetcher"
	"remote-radar/internal/model"
	"remote-radar/internal/notifier"
	"remote-radar/internal/scheduler"
	"remote-radar/internal/storage"
)

// AppConfig 应用配置。
type AppConfig struct {
	Fetcher  fetcher.Config       `yaml:"fetcher"`
	Email    notifier.EmailConfig `yaml:"email"`
	Server   ServerConfig         `yaml:"server"`
	Database DatabaseConfig       `yaml:"database"`
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("load config error: %v", err)
		return
	}

	dbPath := cfg.Database.Path
	if dbPath == "" {
		dbPath = "jobs.db"
	}

	store, err := storage.NewStore(dbPath)
	if err != nil {
		log.Printf("init store error: %v", err)
		return
	}
	defer store.Close()

	client := &http.Client{Timeout: 15 * time.Second}
	baseURL := cfg.Fetcher.BaseURL
	if baseURL == "" {
		baseURL = "https://eleduck.com"
	}
	fetch := fetcher.NewEleduckFetcher(baseURL, cfg.Fetcher, client)
	notif := buildNotifier(cfg.Email)
	sched := scheduler.NewScheduler(fetch, store, notif, scheduler.Config{Interval: cfg.Fetcher.Interval, Timeout: "30s"})

	handler := api.NewHandler(storeAdapter{store}, schedulerAdapter{sched})

	addr := cfg.Server.Addr
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{Addr: addr, Handler: handler}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := sched.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("scheduler stopped: %v", err)
		}
	}()

	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("server error: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
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

func buildNotifier(cfg notifier.EmailConfig) scheduler.Notifier {
	if cfg.Host == "" || cfg.Port == 0 || cfg.From == "" || len(cfg.To) == 0 {
		log.Printf("email notifier disabled: missing host/port/from/to")
		return nil
	}
	return notifier.NewEmailNotifier(cfg, nil)
}

// 适配 API 所需接口。
type storeAdapter struct {
	store *storage.Store
}

func (s storeAdapter) ListJobs(_ *http.Request, limit int) ([]model.Job, error) {
	return s.store.ListJobs(context.Background(), limit)
}

type schedulerAdapter struct {
	sched *scheduler.Scheduler
}

func (s schedulerAdapter) RunOnce(_ *http.Request) (int, error) {
	return s.sched.RunOnce(context.Background())
}
