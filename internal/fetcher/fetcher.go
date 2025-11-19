package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"remote-radar/internal/model"

	"golang.org/x/net/html"
	"gorm.io/datatypes"
)

// Config 定义抓取配置。
type Config struct {
	MaxAgeDays    int      `yaml:"max_age_days" json:"max_age_days"`
	MaxPages      int      `yaml:"max_pages" json:"max_pages"`
	Interval      string   `yaml:"interval" json:"interval"`
	CategoryPaths []string `yaml:"category_paths" json:"category_paths"`
}

// JobFetcher 抓取统一接口。
type JobFetcher interface {
	Fetch(ctx context.Context) ([]model.Job, error)
}

// EleduckFetcher 抓取电鸭职位列表。
type EleduckFetcher struct {
	baseURL       string
	categoryPaths []string
	client        *http.Client
	cfg           Config
	now           func() time.Time
	logger        *log.Logger
}

// NewEleduckFetcher 创建电鸭抓取器，baseURL 形如 https://eleduck.com。
func NewEleduckFetcher(baseURL string, cfg Config, client *http.Client) *EleduckFetcher {
	if client == nil {
		client = http.DefaultClient
	}
	if cfg.MaxPages <= 0 {
		cfg.MaxPages = 1
	}
	if cfg.MaxAgeDays <= 0 {
		cfg.MaxAgeDays = 30
	}
	categoryPaths := normalizeCategoryPaths(cfg.CategoryPaths)
	return &EleduckFetcher{
		baseURL:       strings.TrimSuffix(baseURL, "/"),
		categoryPaths: categoryPaths,
		client:        client,
		cfg:           cfg,
		now:           time.Now,
		logger:        log.New(os.Stdout, "[fetcher] ", log.LstdFlags),
	}
}

// Fetch 抓取最新职位列表，按配置分页与时间窗口限制。
func (e *EleduckFetcher) Fetch(ctx context.Context) ([]model.Job, error) {
	cutoff := e.now().AddDate(0, 0, -e.cfg.MaxAgeDays)
	cutoffText := cutoff.Format(time.RFC3339)

	jobs := make([]model.Job, 0)
	seen := make(map[string]struct{})

	e.logf("start fetch: base=%s categories=%s max_pages=%d max_age_days=%d cutoff=%s", e.baseURL, strings.Join(e.categoryPaths, ","), e.cfg.MaxPages, e.cfg.MaxAgeDays, cutoffText)

	for _, category := range e.categoryPaths {
		stopCategory := false
		for page := 1; page <= e.cfg.MaxPages; page++ {
			pageURL, err := e.buildPageURL(category, page)
			if err != nil {
				return nil, fmt.Errorf("build url: %w", err)
			}
			e.logf("category=%s page=%d url=%s", category, page, pageURL)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
			if err != nil {
				return nil, fmt.Errorf("new request: %w", err)
			}

			resp, err := e.client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("http get: %w", err)
			}
			if resp.Body != nil {
				defer resp.Body.Close()
			}
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("read body: %w", err)
			}

			nextJSON, err := extractNextData(string(body))
			if err != nil {
				return nil, fmt.Errorf("extract __NEXT_DATA__: %w", err)
			}

			posts, err := parseEleduckPosts(nextJSON)
			if err != nil {
				return nil, fmt.Errorf("parse posts: %w", err)
			}
			e.logf("category=%s page=%d parsed_posts=%d", category, page, len(posts))

			pageAccepted := 0
			for _, p := range posts {
				publishedAtText := pickPublishedAt(p)
				if publishedAtText == "" {
					continue // 缺少发布时间，跳过
				}

				publishedAt, err := time.Parse(time.RFC3339, publishedAtText)
				if err != nil || publishedAt.IsZero() {
					continue // 时间格式异常，跳过
				}

				if publishedAt.Before(cutoff) {
					jobID := normalizeID(p.ID)
					e.logf("category=%s page=%d reached_cutoff job_id=%s published_at=%s cutoff=%s", category, page, jobID, publishedAt.Format(time.RFC3339), cutoffText)
					stopCategory = true
					break
				}

				if !hasRemoteTag(p.Tags) {
					continue
				}

				jobID := normalizeID(p.ID)
				if jobID != "" {
					if _, exists := seen[jobID]; exists {
						e.logf("category=%s page=%d skip_duplicate job_id=%s", category, page, jobID)
						continue
					}
					seen[jobID] = struct{}{}
				}

				jobTitle := p.Title
				if jobTitle == "" {
					jobTitle = p.FullTitle
				}
				jobURL := p.URL
				if jobURL == "" && jobID != "" {
					jobURL = "/posts/" + jobID
				}

				job := model.Job{
					ID:            jobID,
					Title:         jobTitle,
					Summary:       pickSummary(p),
					PublishedAt:   publishedAt,
					Source:        "eleduck",
					URL:           e.fullURL(jobURL),
					Tags:          toTagMap(p.Tags),
					RawAttributes: toRawAttributes(p),
				}
				jobs = append(jobs, job)
				pageAccepted++
			}

			e.logf("category=%s page=%d accepted=%d cumulative=%d", category, page, pageAccepted, len(jobs))
			if stopCategory {
				break
			}
		}
	}

	e.logf("fetch done total_jobs=%d", len(jobs))

	return jobs, nil
}

func (e *EleduckFetcher) buildPageURL(categoryPath string, page int) (string, error) {
	base, err := url.Parse(e.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base: %w", err)
	}

	path := categoryPath
	if page > 1 {
		if strings.Contains(path, "?") {
			path = path + fmt.Sprintf("&page=%d", page)
		} else {
			path = path + fmt.Sprintf("?page=%d", page)
		}
	}
	full, err := base.Parse(path)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	return full.String(), nil
}

func (e *EleduckFetcher) fullURL(raw string) string {
	if raw == "" {
		return e.baseURL
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return strings.TrimSuffix(e.baseURL, "/") + raw
}

func (e *EleduckFetcher) logf(format string, args ...any) {
	if e.logger == nil {
		e.logger = log.New(os.Stdout, "[fetcher] ", log.LstdFlags)
	}
	e.logger.Printf(format, args...)
}

func normalizeCategoryPaths(paths []string) []string {
	clean := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return []string{"/categories/5?sort=new", "/categories/22?sort=new"}
	}
	return clean
}

func toRawAttributes(p eleduckPost) datatypes.JSONMap {
	tags := make([]map[string]any, 0, len(p.Tags))
	for _, tag := range p.Tags {
		tags = append(tags, map[string]any{"name": tag.Name})
	}
	return datatypes.JSONMap{
		"id":               p.ID,
		"title":            p.Title,
		"full_title":       p.FullTitle,
		"summary":          p.Summary,
		"excerpt":          p.Excerpt,
		"publishedAt":      p.PublishedAt,
		"published_at":     p.PublishedAtAlt,
		"tags":             tags,
		"url":              p.URL,
		"normalized_title": pickSummary(p),
	}
}

// nextData mirrors __NEXT_DATA__ 结构（精简字段）。
type nextData struct {
	Props struct {
		PageProps    *pageProps    `json:"pageProps"`
		InitialProps *initialProps `json:"initialProps"`
	} `json:"props"`
}

type initialProps struct {
	PageProps *pageProps `json:"pageProps"`
}

type pageProps struct {
	PostList *struct {
		Posts []eleduckPost `json:"posts"`
	} `json:"postList"`
}

type eleduckTag struct {
	Name string `json:"name"`
}

type eleduckPost struct {
	ID             any          `json:"id"`
	Title          string       `json:"title"`
	FullTitle      string       `json:"full_title"`
	Summary        string       `json:"summary"`
	Excerpt        string       `json:"excerpt"`
	PublishedAt    string       `json:"publishedAt"`
	PublishedAtAlt string       `json:"published_at"`
	Tags           []eleduckTag `json:"tags"`
	URL            string       `json:"url"`
}

func extractNextData(htmlText string) (string, error) {
	node, err := html.Parse(strings.NewReader(htmlText))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}

	var scriptText string
	var search func(*html.Node)
	search = func(n *html.Node) {
		if scriptText != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "script" {
			for _, attr := range n.Attr {
				if attr.Key == "id" && attr.Val == "__NEXT_DATA__" {
					if n.FirstChild != nil {
						scriptText = n.FirstChild.Data
					}
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			search(c)
		}
	}
	search(node)

	if scriptText == "" {
		return "", fmt.Errorf("__NEXT_DATA__ not found")
	}
	return scriptText, nil
}

func parseEleduckPosts(jsonText string) ([]eleduckPost, error) {
	var nd nextData
	if err := json.Unmarshal([]byte(jsonText), &nd); err != nil {
		return nil, fmt.Errorf("unmarshal next data: %w", err)
	}

	if nd.Props.PageProps != nil && nd.Props.PageProps.PostList != nil {
		return nd.Props.PageProps.PostList.Posts, nil
	}

	if nd.Props.InitialProps != nil && nd.Props.InitialProps.PageProps != nil && nd.Props.InitialProps.PageProps.PostList != nil {
		return nd.Props.InitialProps.PageProps.PostList.Posts, nil
	}

	return nil, fmt.Errorf("postList not found in __NEXT_DATA__")
}

func hasRemoteTag(tags []eleduckTag) bool {
	for _, t := range tags {
		if strings.Contains(t.Name, "远程") {
			return true
		}
	}
	return false
}

func normalizeID(id any) string {
	switch v := id.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.f", v), ".0"), ".00")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func pickPublishedAt(p eleduckPost) string {
	if p.PublishedAt != "" {
		return p.PublishedAt
	}
	return p.PublishedAtAlt
}

func pickSummary(p eleduckPost) string {
	if p.Summary != "" {
		return p.Summary
	}
	if p.Excerpt != "" {
		return p.Excerpt
	}
	if p.FullTitle != "" {
		return p.FullTitle
	}
	return p.Title
}

func toTagMap(tags []eleduckTag) datatypes.JSONMap {
	m := datatypes.JSONMap{}
	for _, t := range tags {
		m[t.Name] = true
	}
	return m
}
