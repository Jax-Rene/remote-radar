package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"remote-radar/internal/model"
)

// Store 抽象存储接口。
type Store interface {
	ListJobs(r *http.Request, limit, offset int) ([]model.Job, error)
	CountJobs(r *http.Request) (int64, error)
}

// Scheduler 抽象调度接口。
type Scheduler interface {
	RunOnce(r *http.Request) (int, error)
}

// MetaProvider 返回前端元数据。
type MetaProvider interface {
	Snapshot() MetaResponse
}

// SubscriptionService 处理订阅创建。
type SubscriptionService interface {
	Create(ctx context.Context, req SubscriptionRequest) error
}

// MetaResponse 暴露筛选元数据。
type MetaResponse struct {
	TagCandidates   []string `json:"tag_candidates"`
	EmploymentTypes []string `json:"employment_types"`
	SalaryRanges    []string `json:"salary_ranges"`
	RoleCategories  []string `json:"role_categories"`
	LanguageOptions []string `json:"language_options"`
	Channels        []string `json:"channels"`
}

// SubscriptionRequest 表示订阅 API 请求。
type SubscriptionRequest struct {
	Email   string   `json:"email"`
	Channel string   `json:"channel"`
	Tags    []string `json:"tags"`
}

// NewHandler 构造 HTTP 多路复用器。
func NewHandler(store Store, sched Scheduler, meta MetaProvider, subs SubscriptionService) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				if v > 100 {
					v = 100
				}
				limit = v
			}
		}
		page := 1
		if p := r.URL.Query().Get("page"); p != "" {
			if v, err := strconv.Atoi(p); err == nil && v > 0 {
				page = v
			}
		}
		offset := (page - 1) * limit
		fetchLimit := limit + 1

		jobs, err := store.ListJobs(r, fetchLimit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		total, err := store.CountJobs(r)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		hasMore := false
		if len(jobs) > limit {
			hasMore = true
			jobs = jobs[:limit]
		}

		w.Header().Set("X-Page", strconv.Itoa(page))
		w.Header().Set("X-Limit", strconv.Itoa(limit))
		w.Header().Set("X-Has-More", strconv.FormatBool(hasMore))
		w.Header().Set("X-Total", strconv.FormatInt(total, 10))
		writeJSON(w, http.StatusOK, jobs)
	})

	mux.HandleFunc("/api/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		created, err := sched.RunOnce(r)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"created": created})
	})

	mux.HandleFunc("/api/meta", func(w http.ResponseWriter, r *http.Request) {
		var data MetaResponse
		if meta != nil {
			data = meta.Snapshot()
		}
		writeJSON(w, http.StatusOK, data)
	})

	mux.HandleFunc("/api/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if subs == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "subscription disabled"})
			return
		}
		var req SubscriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		if err := subs.Create(r.Context(), req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
	})

	webFS := http.FileServer(http.Dir("web"))
	mux.Handle("/static/", http.StripPrefix("/static/", webFS))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			webFS.ServeHTTP(w, r)
			return
		}
		path := filepath.Clean("web/index.html")
		data, err := os.ReadFile(path)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]string{"message": "remote jobs api"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
