package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"remote-radar/internal/model"
)

func TestListJobs(t *testing.T) {
	t.Parallel()

	st := &stubStore{jobs: []model.Job{{ID: "1", Title: "Backend"}, {ID: "2", Title: "FE"}}, total: 42}
	sch := &stubScheduler{}

	h := NewHandler(st, sch)
	req := httptest.NewRequest(http.MethodGet, "/api/jobs?limit=1&page=1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if st.calls != 1 {
		t.Fatalf("expected store called once, got %d", st.calls)
	}
	if st.countCalls != 1 {
		t.Fatalf("expected count called once, got %d", st.countCalls)
	}
	if st.lastLimit != 2 {
		t.Fatalf("expected fetch limit 2, got %d", st.lastLimit)
	}
	if st.lastOffset != 0 {
		t.Fatalf("expected offset 0, got %d", st.lastOffset)
	}
	if w.Header().Get("X-Has-More") != "true" {
		t.Fatalf("expected has-more true header")
	}
	if w.Header().Get("X-Total") != "42" {
		t.Fatalf("expected total header 42, got %s", w.Header().Get("X-Total"))
	}
	var resp []model.Job
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(resp) != 1 || resp[0].ID != "1" {
		t.Fatalf("expected single trimmed job, got %+v", resp)
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
	jobs       []model.Job
	calls      int
	lastLimit  int
	lastOffset int
	countCalls int
	total      int64
}

func (s *stubStore) ListJobs(r *http.Request, limit, offset int) ([]model.Job, error) {
	s.calls++
	s.lastLimit = limit
	s.lastOffset = offset
	return s.jobs, nil
}

func (s *stubStore) CountJobs(r *http.Request) (int64, error) {
	s.countCalls++
	return s.total, nil
}

type stubScheduler struct {
	created int
	calls   int
}

func (s *stubScheduler) RunOnce(r *http.Request) (int, error) {
	s.calls++
	return s.created, nil
}
