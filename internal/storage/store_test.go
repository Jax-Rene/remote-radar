package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"remote-radar/internal/model"

	"gorm.io/datatypes"
)

func TestStoreUpsertAndList(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "jobs.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	ctx := context.Background()
	first := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	second := first.Add(2 * time.Hour)

	jobs := []model.Job{
		{
			ID:          "1",
			Title:       "Backend Engineer",
			Summary:     "Go remote role",
			PublishedAt: first,
			Source:      "eleduck",
			URL:         "https://example.com/1",
			Tags:        datatypes.JSONMap{"mode": "远程工作"},
			RawAttributes: datatypes.JSONMap{
				"origin_id": "1",
			},
		},
		{
			ID:          "2",
			Title:       "Frontend Engineer",
			Summary:     "React remote role",
			PublishedAt: second,
			Source:      "eleduck",
			URL:         "https://example.com/2",
			Tags:        datatypes.JSONMap{"mode": "远程工作"},
			RawAttributes: datatypes.JSONMap{
				"origin_id": "2",
			},
		},
	}

	res, err := store.UpsertJobs(ctx, jobs)
	if err != nil {
		t.Fatalf("UpsertJobs error: %v", err)
	}
	if res.Created != 2 {
		t.Fatalf("expected 2 created jobs, got %d", res.Created)
	}
	if len(res.NewJobs) != 2 {
		t.Fatalf("expected 2 new jobs returned, got %d", len(res.NewJobs))
	}

	jobs[1].Title = "Senior Frontend Engineer"
	res, err = store.UpsertJobs(ctx, jobs)
	if err != nil {
		t.Fatalf("UpsertJobs second run error: %v", err)
	}
	if res.Created != 0 {
		t.Fatalf("expected 0 newly created jobs on second upsert, got %d", res.Created)
	}

	got, err := store.ListJobs(ctx, JobQueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(got))
	}
	if got[0].ID != "2" {
		t.Fatalf("expected most recent job ID '2', got %s", got[0].ID)
	}
	if got[0].Title != "Senior Frontend Engineer" {
		t.Fatalf("expected updated title to persist, got %s", got[0].Title)
	}
	if got[0].RawAttributes["origin_id"] != "2" {
		t.Fatalf("expected raw attributes stored for latest job, got %#v", got[0].RawAttributes)
	}

	paged, err := store.ListJobs(ctx, JobQueryOptions{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("ListJobs with offset error: %v", err)
	}
	if len(paged) != 1 || paged[0].ID != "1" {
		t.Fatalf("expected second job when offset=1, got %+v", paged)
	}

	total, err := store.CountJobs(ctx, JobQueryOptions{})
	if err != nil {
		t.Fatalf("CountJobs error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2 jobs, got %d", total)
	}
}

func TestGetJobByID(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "jobs.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	job := model.Job{
		ID:          "abc",
		Title:       "Data Engineer",
		Summary:     "Pipeline work",
		PublishedAt: time.Now(),
		Source:      "eleduck",
		URL:         "https://example.com/abc",
		Tags:        datatypes.JSONMap{"mode": "远程工作"},
		RawAttributes: datatypes.JSONMap{
			"origin_id": "abc",
		},
	}

	res, err := store.UpsertJobs(ctx, []model.Job{job})
	if err != nil {
		t.Fatalf("UpsertJobs error: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("expected 1 created job, got %d", res.Created)
	}

	fetched, err := store.GetJob(ctx, "abc")
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	if fetched == nil {
		t.Fatalf("expected job to exist")
	}
	if fetched.Title != job.Title {
		t.Fatalf("expected title %s, got %s", job.Title, fetched.Title)
	}
	if fetched.RawAttributes["origin_id"] != "abc" {
		t.Fatalf("expected raw attributes stored, got %#v", fetched.RawAttributes)
	}
}

func TestListJobsFiltersByNormalizedTags(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	store, err := NewStore(filepath.Join(tmp, "jobs.db"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	jobs := []model.Job{
		{ID: "tag1", Title: "Backend", PublishedAt: time.Now(), Source: "eleduck", NormalizedTags: datatypes.JSONMap{"backend": true, "go": true}},
		{ID: "tag2", Title: "Frontend", PublishedAt: time.Now().Add(-time.Hour), Source: "eleduck", NormalizedTags: datatypes.JSONMap{"frontend": true}},
	}
	if _, err := store.UpsertJobs(ctx, jobs); err != nil {
		t.Fatalf("UpsertJobs error: %v", err)
	}

	filtered, err := store.ListJobs(ctx, JobQueryOptions{Tags: []string{"backend"}})
	if err != nil {
		t.Fatalf("ListJobs filter error: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "tag1" {
		t.Fatalf("expected backend job returned, got %+v", filtered)
	}

	total, err := store.CountJobs(ctx, JobQueryOptions{Tags: []string{"frontend"}})
	if err != nil {
		t.Fatalf("CountJobs filter error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1 for frontend filter, got %d", total)
	}
}

func TestRawJobLifecycle(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	store, err := NewStore(filepath.Join(tmp, "jobs.db"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	res, err := store.UpsertRawJobs(ctx, []model.RawJob{{Source: "eleduck", ExternalID: "raw-1", Title: "Raw", PublishedAt: time.Now()}})
	if err != nil {
		t.Fatalf("UpsertRawJobs error: %v", err)
	}
	if res.Created != 1 || len(res.NewJobs) != 1 {
		t.Fatalf("expected 1 new raw job, got %+v", res)
	}

	pending, err := store.ListRawJobs(ctx, RawJobQuery{Status: model.RawJobStatusPending, Limit: 10})
	if err != nil {
		t.Fatalf("ListRawJobs error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected pending job, got %+v", pending)
	}

	update := RawJobStatusUpdate{Status: model.RawJobStatusProcessed, Details: datatypes.JSONMap{"score": 5}}
	if err := store.UpdateRawJobStatus(ctx, pending[0].ID, update); err != nil {
		t.Fatalf("UpdateRawJobStatus error: %v", err)
	}

	pending, err = store.ListRawJobs(ctx, RawJobQuery{Status: model.RawJobStatusPending})
	if err != nil {
		t.Fatalf("ListRawJobs after update error: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending jobs, got %+v", pending)
	}
}

func TestSubscriptionCreateAndList(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	store, err := NewStore(filepath.Join(tmp, "jobs.db"))
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	sub := model.Subscription{Email: "user@example.com", Channel: "email", Tags: datatypes.JSONMap{"backend": true}}
	if err := store.CreateSubscription(ctx, &sub); err != nil {
		t.Fatalf("CreateSubscription error: %v", err)
	}
	if sub.ID == 0 {
		t.Fatalf("expected subscription ID assigned")
	}

	subs, err := store.ListSubscriptions(ctx)
	if err != nil {
		t.Fatalf("ListSubscriptions error: %v", err)
	}
	if len(subs) != 1 || subs[0].Email != sub.Email {
		t.Fatalf("expected stored subscription returned, got %+v", subs)
	}
}
