package fetcher

import (
	"context"
	"encoding/json"
	"io"
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
		baseURL:      "http://example.com",
		categoryPath: "/categories/5?sort=new",
		client:       &http.Client{Transport: rt},
		cfg:          cfg,
		now:          func() time.Time { return now },
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
		baseURL:      "http://example.com",
		categoryPath: "/categories/5?sort=new",
		client:       &http.Client{Transport: rt},
		cfg:          cfg,
		now:          func() time.Time { return now },
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
