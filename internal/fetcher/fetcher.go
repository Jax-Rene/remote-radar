package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"remote-radar/internal/model"

	"golang.org/x/net/html"
	"gorm.io/datatypes"
)

// Config 定义抓取配置。
type Config struct {
	MaxAgeDays int    `yaml:"max_age_days" json:"max_age_days"`
	MaxPages   int    `yaml:"max_pages" json:"max_pages"`
	Interval   string `yaml:"interval" json:"interval"`
}

// JobFetcher 抓取统一接口。
type JobFetcher interface {
	Fetch(ctx context.Context) ([]model.Job, error)
}

// EleduckFetcher 抓取电鸭职位列表。
type EleduckFetcher struct {
	baseURL      string
	categoryPath string
	client       *http.Client
	cfg          Config
	now          func() time.Time
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
	return &EleduckFetcher{
		baseURL:      strings.TrimSuffix(baseURL, "/"),
		categoryPath: "/categories/5?sort=new",
		client:       client,
		cfg:          cfg,
		now:          time.Now,
	}
}

// Fetch 抓取最新职位列表，按配置分页与时间窗口限制。
func (e *EleduckFetcher) Fetch(ctx context.Context) ([]model.Job, error) {
	cutoff := e.now().AddDate(0, 0, -e.cfg.MaxAgeDays)

	jobs := make([]model.Job, 0)

	for page := 1; page <= e.cfg.MaxPages; page++ {
		pageURL, err := e.buildPageURL(page)
		if err != nil {
			return nil, fmt.Errorf("build url: %w", err)
		}

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

		stop := false
		for _, p := range posts {
			publishedAt, err := time.Parse(time.RFC3339, p.PublishedAt)
			if err != nil {
				return nil, fmt.Errorf("parse time: %w", err)
			}

			if publishedAt.Before(cutoff) {
				stop = true
				break
			}

			if !hasRemoteTag(p.Tags) {
				continue
			}

			job := model.Job{
				ID:          normalizeID(p.ID),
				Title:       p.Title,
				Summary:     pickSummary(p),
				PublishedAt: publishedAt,
				Source:      "eleduck",
				URL:         e.fullURL(p.URL),
				Tags:        toTagMap(p.Tags),
			}
			jobs = append(jobs, job)
		}

		if stop {
			break
		}
	}

	return jobs, nil
}

func (e *EleduckFetcher) buildPageURL(page int) (string, error) {
	base, err := url.Parse(e.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base: %w", err)
	}

	path := e.categoryPath
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

// nextData mirrors __NEXT_DATA__ 结构（精简字段）。
type nextData struct {
	Props struct {
		PageProps struct {
			PostList struct {
				Posts []eleduckPost `json:"posts"`
			} `json:"postList"`
		} `json:"pageProps"`
	} `json:"props"`
}

type eleduckTag struct {
	Name string `json:"name"`
}

type eleduckPost struct {
	ID          any          `json:"id"`
	Title       string       `json:"title"`
	Summary     string       `json:"summary"`
	Excerpt     string       `json:"excerpt"`
	PublishedAt string       `json:"publishedAt"`
	Tags        []eleduckTag `json:"tags"`
	URL         string       `json:"url"`
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
	return nd.Props.PageProps.PostList.Posts, nil
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

func pickSummary(p eleduckPost) string {
	if p.Summary != "" {
		return p.Summary
	}
	if p.Excerpt != "" {
		return p.Excerpt
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
