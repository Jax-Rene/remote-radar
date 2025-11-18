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
		},
		{
			ID:          "2",
			Title:       "Frontend Engineer",
			Summary:     "React remote role",
			PublishedAt: second,
			Source:      "eleduck",
			URL:         "https://example.com/2",
			Tags:        datatypes.JSONMap{"mode": "远程工作"},
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

	// Re-upsert with updated title to ensure we update existing rows but do not count as new.
	jobs[1].Title = "Senior Frontend Engineer"
	res, err = store.UpsertJobs(ctx, jobs)
	if err != nil {
		t.Fatalf("UpsertJobs second run error: %v", err)
	}
	if res.Created != 0 {
		t.Fatalf("expected 0 newly created jobs on second upsert, got %d", res.Created)
	}

	got, err := store.ListJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ListJobs error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(got))
	}
	if got[0].ID != "2" { // should be ordered by PublishedAt desc
		t.Fatalf("expected most recent job ID '2', got %s", got[0].ID)
	}
	if got[0].Title != "Senior Frontend Engineer" {
		t.Fatalf("expected updated title to persist, got %s", got[0].Title)
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
}
