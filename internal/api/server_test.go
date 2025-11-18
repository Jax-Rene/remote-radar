package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"remote-radar/internal/model"
)

func TestListJobs(t *testing.T) {
	t.Parallel()

	st := &stubStore{jobs: []model.Job{{ID: "1", Title: "Backend"}}}
	sch := &stubScheduler{}

	h := NewHandler(st, sch)
	req := httptest.NewRequest(http.MethodGet, "/api/jobs?limit=5", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if st.calls != 1 {
		t.Fatalf("expected store called once, got %d", st.calls)
	}
}

func TestRefresh(t *testing.T) {
	t.Parallel()

	st := &stubStore{}
	sch := &stubScheduler{created: 2}

	h := NewHandler(st, sch)
	req := httptest.NewRequest(http.MethodPost, "/api/refresh", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if sch.calls != 1 {
		t.Fatalf("expected scheduler called once, got %d", sch.calls)
	}
}

// --- stubs ---

type stubStore struct {
	jobs  []model.Job
	calls int
}

func (s *stubStore) ListJobs(r *http.Request, limit int) ([]model.Job, error) {
	s.calls++
	return s.jobs, nil
}

type stubScheduler struct {
	created int
	calls   int
}

func (s *stubScheduler) RunOnce(r *http.Request) (int, error) {
	s.calls++
	return s.created, nil
}
