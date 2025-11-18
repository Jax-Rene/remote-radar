package api

import (
    "encoding/json"
    "net/http"
    "os"
    "path/filepath"
    "strconv"

    "remote-radar/internal/model"
)

// Store 抽象存储接口。
type Store interface {
	ListJobs(r *http.Request, limit int) ([]model.Job, error)
}

// Scheduler 抽象调度接口。
type Scheduler interface {
	RunOnce(r *http.Request) (int, error)
}

// NewHandler 构造 HTTP 多路复用器。
func NewHandler(store Store, sched Scheduler) http.Handler {
    mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		limit := 20
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}
		jobs, err := store.ListJobs(r, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
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

    mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/" {
            w.WriteHeader(http.StatusNotFound)
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
