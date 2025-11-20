package api

import (
	"bytes"
	"context"
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
	meta := &stubMetaProvider{}
	subscriber := &stubSubscriber{}

	h := NewHandler(st, sch, meta, subscriber)
	req := httptest.NewRequest(http.MethodGet, "/api/jobs?limit=1&page=1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if st.calls != 1 || st.countCalls != 1 {
		t.Fatalf("unexpected store call counts %+v", st)
	}
	if st.lastLimit != 2 || st.lastOffset != 0 {
		t.Fatalf("unexpected list args: limit=%d offset=%d", st.lastLimit, st.lastOffset)
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
	meta := &stubMetaProvider{}
	subscriber := &stubSubscriber{}

	h := NewHandler(st, sch, meta, subscriber)
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

func TestCreateSubscription(t *testing.T) {
	t.Parallel()

	h := NewHandler(&stubStore{}, &stubScheduler{}, &stubMetaProvider{}, &stubSubscriber{})
	req := httptest.NewRequest(http.MethodPost, "/api/subscriptions", bytes.NewBufferString(`{"email":"a@b.com","channel":"email","tags":["backend"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestMetaEndpoint(t *testing.T) {
	t.Parallel()

	meta := &stubMetaProvider{data: MetaResponse{TagCandidates: []string{"backend"}}}
	h := NewHandler(&stubStore{}, &stubScheduler{}, meta, &stubSubscriber{})
	req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp MetaResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	if len(resp.TagCandidates) != 1 || resp.TagCandidates[0] != "backend" {
		t.Fatalf("unexpected meta response: %+v", resp)
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

type stubSubscriber struct {
	calls int
}

func (s *stubSubscriber) Create(ctx context.Context, req SubscriptionRequest) error {
	s.calls++
	return nil
}

type stubMetaProvider struct {
	data MetaResponse
}

func (m *stubMetaProvider) Snapshot() MetaResponse { return m.data }
