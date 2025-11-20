package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DeepseekConfig 定义 DeepSeek API 配置。
type DeepseekConfig struct {
	APIBase string `yaml:"api_base" json:"api_base"`
	APIKey  string `yaml:"api_key" json:"api_key"`
	Model   string `yaml:"model" json:"model"`
}

// DeepseekClient 实现 LLMClient。
type DeepseekClient struct {
	cfg    DeepseekConfig
	client *http.Client
}

// NewDeepseekClient 创建客户端。
func NewDeepseekClient(cfg DeepseekConfig, httpClient *http.Client) *DeepseekClient {
	base := strings.TrimSpace(cfg.APIBase)
	if base == "" {
		base = "https://api.deepseek.com/v1"
	}
	model := cfg.Model
	if model == "" {
		model = "deepseek-chat"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &DeepseekClient{cfg: DeepseekConfig{APIBase: base, APIKey: cfg.APIKey, Model: model}, client: httpClient}
}

func (c *DeepseekClient) Complete(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return "", fmt.Errorf("deepseek api key missing")
	}

	payload := deepseekRequest{
		Model: c.cfg.Model,
		Messages: []deepseekMessage{
			{Role: "system", Content: "You are a helpful talent acquisition assistant."},
			{Role: "user", Content: prompt},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.APIBase, "/")+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("deepseek request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("deepseek http %d", resp.StatusCode)
	}

	var body deepseekResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode deepseek response: %w", err)
	}

	if len(body.Choices) == 0 || body.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("deepseek response empty")
	}

	return strings.TrimSpace(body.Choices[0].Message.Content), nil
}

type deepseekRequest struct {
	Model    string            `json:"model"`
	Messages []deepseekMessage `json:"messages"`
}

type deepseekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepseekResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
