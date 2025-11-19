package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEleduckFetchFiltersByTagAndAge(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
	maxAgeDays := 30

	page1HTML := buildEleduckHTML([]postFixture{
		{
			ID:          "101",
			Title:       "Remote Backend",
			Summary:     "Go role",
			PublishedAt: now.Add(-24 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/101",
		},
		{
			ID:          "102",
			Title:       "Onsite QA",
			Summary:     "Not remote",
			PublishedAt: now.Add(-24 * time.Hour),
			Tags:        []string{"全职"},
			URL:         "/post/102",
		},
		{
			ID:          "103",
			Title:       "Old Remote",
			Summary:     "Too old",
			PublishedAt: now.AddDate(0, 0, -45),
			Tags:        []string{"远程工作"},
			URL:         "/post/103",
		},
	})

	page2URL := "http://example.com/categories/5?sort=new&page=2"
	pageHits := &atomic.Int32{}
	rt := newStubRoundTripper(map[string]string{
		"http://example.com/categories/5?sort=new": page1HTML,
		page2URL: buildEleduckHTML(nil),
	}, pageHits)

	cfg := Config{MaxAgeDays: maxAgeDays, MaxPages: 3}
	fetcher := &EleduckFetcher{
		baseURL:       "http://example.com",
		categoryPaths: []string{"/categories/5?sort=new"},
		client:        &http.Client{Transport: rt},
		cfg:           cfg,
		now:           func() time.Time { return now },
	}

	jobs, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}

	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	job := jobs[0]
	if job.ID != "101" {
		t.Fatalf("expected job ID 101, got %s", job.ID)
	}

	if pageHits.Load() != 1 {
		t.Fatalf("expected no request for page 2 when encountering old post, got %d hits", pageHits.Load())
	}
}

func TestEleduckFetchPaginates(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)

	pageHits := &atomic.Int32{}

	rt := newStubRoundTripper(map[string]string{
		"http://example.com/categories/5?sort=new": buildEleduckHTML([]postFixture{{
			ID:          "201",
			Title:       "Remote FE",
			Summary:     "React",
			PublishedAt: now.Add(-12 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/201",
		}}),
		"http://example.com/categories/5?sort=new&page=2": buildEleduckHTML([]postFixture{{
			ID:          "202",
			Title:       "Remote DevOps",
			Summary:     "Cloud",
			PublishedAt: now.Add(-36 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/202",
		}}),
	}, pageHits)

	cfg := Config{MaxPages: 2, MaxAgeDays: 365}
	fetcher := &EleduckFetcher{
		baseURL:       "http://example.com",
		categoryPaths: []string{"/categories/5?sort=new"},
		client:        &http.Client{Transport: rt},
		cfg:           cfg,
		now:           func() time.Time { return now },
	}

	jobs, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}

	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	var ids []string
	for _, j := range jobs {
		ids = append(ids, j.ID)
	}
	got := strings.Join(ids, ",")
	if got != "201,202" {
		t.Fatalf("unexpected job IDs order: %s", got)
	}

	if pageHits.Load() != 2 {
		t.Fatalf("expected two page hits, got %d", pageHits.Load())
	}
}

func TestEleduckFetchIncludesTalentCategory(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 6, 11, 0, 0, 0, 0, time.UTC)

	rt := newStubRoundTripper(map[string]string{
		"http://example.com/categories/5?sort=new": buildEleduckHTML([]postFixture{{
			ID:          "201",
			Title:       "Remote FE",
			Summary:     "React",
			PublishedAt: now.Add(-12 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/201",
		}}),
		"http://example.com/categories/22?sort=new": buildEleduckHTML([]postFixture{{
			ID:          "talent",
			Title:       "Talent Pool",
			Summary:     "Remote talent",
			PublishedAt: now.Add(-6 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/talent",
		}}),
	}, &atomic.Int32{})

	client := &http.Client{Transport: rt}
	cfg := Config{MaxPages: 1, MaxAgeDays: 365}
	fetcher := NewEleduckFetcher("http://example.com", cfg, client)
	fetcher.now = func() time.Time { return now }

	jobs, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs from both categories, got %d", len(jobs))
	}

	got := map[string]bool{}
	for _, job := range jobs {
		got[job.ID] = true
	}
	if !got["201"] || !got["talent"] {
		t.Fatalf("expected IDs 201 and talent, got %#v", got)
	}
}

func TestEleduckFetchDeduplicatesAcrossCategories(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)

	rt := newStubRoundTripper(map[string]string{
		"http://example.com/categories/5?sort=new": buildEleduckHTML([]postFixture{{
			ID:          "dup",
			Title:       "Remote Role",
			Summary:     "Go",
			PublishedAt: now.Add(-12 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/dup",
		}}),
		"http://example.com/categories/22?sort=new": buildEleduckHTML([]postFixture{{
			ID:          "dup",
			Title:       "Talent Duplicate",
			Summary:     "Remote talent",
			PublishedAt: now.Add(-2 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/talent-dup",
		}}),
	}, &atomic.Int32{})

	client := &http.Client{Transport: rt}
	cfg := Config{MaxPages: 1, MaxAgeDays: 365}
	fetcher := NewEleduckFetcher("http://example.com", cfg, client)
	fetcher.now = func() time.Time { return now }

	jobs, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected duplicate ID filtered once, got %d jobs", len(jobs))
	}
	if jobs[0].ID != "dup" {
		t.Fatalf("expected job ID dup, got %s", jobs[0].ID)
	}
}

func TestEleduckFetchLogsStageStatistics(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
	pageHTML := buildEleduckHTML([]postFixture{
		{
			ID:          "keep",
			Title:       "Remote Role",
			Summary:     "Go dev",
			PublishedAt: now.Add(-2 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/keep",
		},
		{
			ID:          "old",
			Title:       "Old Remote",
			Summary:     "Too old",
			PublishedAt: now.AddDate(0, 0, -45),
			Tags:        []string{"远程工作"},
			URL:         "/post/old",
		},
	})

	rt := newStubRoundTripper(map[string]string{
		"http://example.com/categories/5?sort=new": pageHTML,
	}, &atomic.Int32{})

	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	fetcher := &EleduckFetcher{
		baseURL:       "http://example.com",
		categoryPaths: []string{"/categories/5?sort=new"},
		client:        &http.Client{Transport: rt},
		cfg:           Config{MaxPages: 2, MaxAgeDays: 30},
		now:           func() time.Time { return now },
		logger:        logger,
	}

	jobs, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	logs := buf.String()
	for _, want := range []string{
		"start fetch",
		"category=/categories/5?sort=new page=1 url=http://example.com/categories/5?sort=new",
		"category=/categories/5?sort=new page=1 parsed_posts=2",
		"category=/categories/5?sort=new page=1 accepted=1 cumulative=1",
		"category=/categories/5?sort=new page=1 reached_cutoff job_id=old",
		"fetch done total_jobs=1",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected logs to contain %q, got logs: %s", want, logs)
		}
	}
}

func TestEleduckFetchSupportsInitialPropsShape(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 11, 19, 0, 0, 0, 0, time.UTC)

	rt := newStubRoundTripper(map[string]string{
		"http://example.com/categories/5?sort=new": buildEleduckHTMLWithInitialProps([]postFixture{{
			ID:          "301",
			Title:       "Remote Backend New Shape",
			Summary:     "Go",
			PublishedAt: now.Add(-12 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/301",
		}}),
	}, &atomic.Int32{})

	cfg := Config{MaxPages: 1, MaxAgeDays: 365}
	fetcher := &EleduckFetcher{
		baseURL:       "http://example.com",
		categoryPaths: []string{"/categories/5?sort=new"},
		client:        &http.Client{Transport: rt},
		cfg:           cfg,
		now:           func() time.Time { return now },
	}

	jobs, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}

	if len(jobs) != 1 || jobs[0].ID != "301" {
		t.Fatalf("unexpected jobs: %+v", jobs)
	}
}

func TestEleduckFetchSkipsInvalidPublishedAt(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 11, 19, 0, 0, 0, 0, time.UTC)

	rt := newStubRoundTripper(map[string]string{
		"http://example.com/categories/5?sort=new": buildEleduckHTML([]postFixture{{
			ID:          "bad",
			Title:       "Broken",
			Summary:     "Missing time",
			PublishedAt: time.Time{},
			Tags:        []string{"远程工作"},
			URL:         "/post/bad",
		}, {
			ID:          "good",
			Title:       "Valid",
			Summary:     "Has time",
			PublishedAt: now.Add(-2 * time.Hour),
			Tags:        []string{"远程工作"},
			URL:         "/post/good",
		}}),
	}, &atomic.Int32{})

	fetcher := &EleduckFetcher{
		baseURL:       "http://example.com",
		categoryPaths: []string{"/categories/5?sort=new"},
		client:        &http.Client{Transport: rt},
		cfg:           Config{MaxPages: 1, MaxAgeDays: 365},
		now:           func() time.Time { return now },
	}

	jobs, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}

	if len(jobs) != 1 || jobs[0].ID != "good" {
		t.Fatalf("expected only valid job, got: %+v", jobs)
	}
}

func TestEleduckFetchHandlesSnakeCasePayload(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 11, 20, 12, 0, 0, 0, time.UTC)

	rt := newStubRoundTripper(map[string]string{
		"http://example.com/categories/5?sort=new": buildEleduckHTMLSnakeCase([]postFixture{{
			ID:          "snake",
			Title:       "Remote Snake Case",
			Summary:     "snake summary",
			PublishedAt: now.Add(-4 * time.Hour),
			Tags:        []string{"远程工作"},
		}}),
	}, &atomic.Int32{})

	fetcher := &EleduckFetcher{
		baseURL:       "http://example.com",
		categoryPaths: []string{"/categories/5?sort=new"},
		client:        &http.Client{Transport: rt},
		cfg:           Config{MaxPages: 1, MaxAgeDays: 30},
		now:           func() time.Time { return now },
	}

	jobs, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	job := jobs[0]
	expectedTime := now.Add(-4 * time.Hour)
	if !job.PublishedAt.Equal(expectedTime) {
		t.Fatalf("expected published time %v, got %v", expectedTime, job.PublishedAt)
	}

	wantURL := "http://example.com/posts/snake"
	if job.URL != wantURL {
		t.Fatalf("expected URL %s, got %s", wantURL, job.URL)
	}
}

func TestEleduckFetchCapturesRawAttributes(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 11, 20, 12, 0, 0, 0, time.UTC)

	rt := newStubRoundTripper(map[string]string{
		"http://example.com/categories/5?sort=new": buildEleduckHTML([]postFixture{{
			ID:          "raw",
			Title:       "Raw Role",
			Summary:     "Contains raw data",
			PublishedAt: now.Add(-4 * time.Hour),
			Tags:        []string{"远程工作", "远程人才"},
			URL:         "/post/raw",
		}}),
	}, &atomic.Int32{})

	cfg := Config{MaxPages: 1, MaxAgeDays: 30, CategoryPaths: []string{"/categories/5?sort=new"}}
	fetcher := NewEleduckFetcher("http://example.com", cfg, &http.Client{Transport: rt})
	fetcher.now = func() time.Time { return now }

	jobs, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	raw := jobs[0].RawAttributes
	if raw == nil {
		t.Fatalf("expected raw attributes stored, got nil")
	}
	if raw["title"] != "Raw Role" {
		t.Fatalf("expected raw title preserved, got %#v", raw["title"])
	}
	switch tags := raw["tags"].(type) {
	case []any:
		if len(tags) == 0 {
			t.Fatalf("expected tags array in raw attributes, got %#v", raw["tags"])
		}
	case []map[string]any:
		if len(tags) == 0 {
			t.Fatalf("expected tags array in raw attributes, got %#v", raw["tags"])
		}
	default:
		t.Fatalf("expected slice tags in raw attributes, got %T", raw["tags"])
	}
}

func TestEleduckFetchRealManual(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 15 * time.Second}
	cfg := Config{MaxPages: 1, MaxAgeDays: 7}
	fetcher := NewEleduckFetcher("https://eleduck.com", cfg, client)

	jobs, err := fetcher.Fetch(ctx)
	if err != nil {
		t.Fatalf("failed to fetch real jobs: %v", err)
	}

	if len(jobs) == 0 {
		t.Fatalf("expected at least one job from eleduck real fetch")
	}

	job := jobs[0]
	if job.Source != "eleduck" || job.Title == "" || job.URL == "" {
		t.Fatalf("job missing required fields: %+v", job)
	}

	t.Logf("Fetched %d jobs from Eleduck, sample job: %+v", len(jobs), job)
}

type postFixture struct {
	ID          string
	Title       string
	Summary     string
	PublishedAt time.Time
	Tags        []string
	URL         string
}

func buildEleduckHTML(posts []postFixture) string {
	type tagPayload struct {
		Name string `json:"name"`
	}
	type postPayload struct {
		ID          string       `json:"id"`
		Title       string       `json:"title"`
		Summary     string       `json:"summary"`
		PublishedAt string       `json:"publishedAt"`
		Tags        []tagPayload `json:"tags"`
		URL         string       `json:"url"`
	}
	type nextData struct {
		Props struct {
			PageProps struct {
				PostList struct {
					Posts []postPayload `json:"posts"`
				} `json:"postList"`
			} `json:"pageProps"`
		} `json:"props"`
	}

	payload := nextData{}
	for _, p := range posts {
		tags := make([]tagPayload, 0, len(p.Tags))
		for _, tag := range p.Tags {
			tags = append(tags, tagPayload{Name: tag})
		}
		payload.Props.PageProps.PostList.Posts = append(payload.Props.PageProps.PostList.Posts, postPayload{
			ID:          p.ID,
			Title:       p.Title,
			Summary:     p.Summary,
			PublishedAt: p.PublishedAt.Format(time.RFC3339),
			Tags:        tags,
			URL:         p.URL,
		})
	}

	jsonBytes, _ := json.Marshal(payload)
	return "<html><head></head><body><script id=\"__NEXT_DATA__\" type=\"application/json\">" + string(jsonBytes) + "</script></body></html>"
}

func buildEleduckHTMLWithInitialProps(posts []postFixture) string {
	type tagPayload struct {
		Name string `json:"name"`
	}
	type postPayload struct {
		ID          string       `json:"id"`
		Title       string       `json:"title"`
		Summary     string       `json:"summary"`
		PublishedAt string       `json:"publishedAt"`
		Tags        []tagPayload `json:"tags"`
		URL         string       `json:"url"`
	}
	type nextData struct {
		Props struct {
			InitialProps struct {
				PageProps struct {
					PostList struct {
						Posts []postPayload `json:"posts"`
					} `json:"postList"`
				} `json:"pageProps"`
			} `json:"initialProps"`
		} `json:"props"`
	}

	payload := nextData{}
	for _, p := range posts {
		tags := make([]tagPayload, 0, len(p.Tags))
		for _, tag := range p.Tags {
			tags = append(tags, tagPayload{Name: tag})
		}
		payload.Props.InitialProps.PageProps.PostList.Posts = append(payload.Props.InitialProps.PageProps.PostList.Posts, postPayload{
			ID:          p.ID,
			Title:       p.Title,
			Summary:     p.Summary,
			PublishedAt: p.PublishedAt.Format(time.RFC3339),
			Tags:        tags,
			URL:         p.URL,
		})
	}

	jsonBytes, _ := json.Marshal(payload)
	return "<html><head></head><body><script id=\"__NEXT_DATA__\" type=\"application/json\">" + string(jsonBytes) + "</script></body></html>"
}

func buildEleduckHTMLSnakeCase(posts []postFixture) string {
	type tagPayload struct {
		Name string `json:"name"`
	}
	type postPayload struct {
		ID          string       `json:"id"`
		Title       string       `json:"title"`
		FullTitle   string       `json:"full_title"`
		Summary     string       `json:"summary"`
		PublishedAt string       `json:"published_at"`
		Tags        []tagPayload `json:"tags"`
	}
	type nextData struct {
		Props struct {
			InitialProps struct {
				PageProps struct {
					PostList struct {
						Posts []postPayload `json:"posts"`
					} `json:"postList"`
				} `json:"pageProps"`
			} `json:"initialProps"`
		} `json:"props"`
	}

	payload := nextData{}
	for _, p := range posts {
		tags := make([]tagPayload, 0, len(p.Tags))
		for _, tag := range p.Tags {
			tags = append(tags, tagPayload{Name: tag})
		}
		payload.Props.InitialProps.PageProps.PostList.Posts = append(payload.Props.InitialProps.PageProps.PostList.Posts, postPayload{
			ID:          p.ID,
			Title:       p.Title,
			FullTitle:   p.Title,
			Summary:     p.Summary,
			PublishedAt: p.PublishedAt.Format(time.RFC3339),
			Tags:        tags,
		})
	}

	jsonBytes, _ := json.Marshal(payload)
	return "<html><head></head><body><script id=\"__NEXT_DATA__\" type=\"application/json\">" + string(jsonBytes) + "</script></body></html>"
}

type stubRoundTripper struct {
	responses map[string]string
	hits      *atomic.Int32
	mu        sync.Mutex
}

func newStubRoundTripper(responses map[string]string, hits *atomic.Int32) *stubRoundTripper {
	return &stubRoundTripper{responses: responses, hits: hits}
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.hits.Add(1)
	body, ok := s.responses[req.URL.String()]
	s.mu.Unlock()
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("not found")),
			Header:     make(http.Header),
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}
