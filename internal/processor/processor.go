package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"remote-radar/internal/model"

	"gorm.io/datatypes"
)

// Config 描述清洗与 LLM 提示词配置。
type Config struct {
	Keywords        []string       `yaml:"keywords" json:"keywords"`
	PromptTemplate  string         `yaml:"prompt_template" json:"prompt_template"`
	TagCandidates   []string       `yaml:"tag_candidates" json:"tag_candidates"`
	EmploymentTypes []string       `yaml:"employment_types" json:"employment_types"`
	SalaryRanges    []string       `yaml:"salary_ranges" json:"salary_ranges"`
	RoleCategories  []string       `yaml:"role_categories" json:"role_categories"`
	LanguageOptions []string       `yaml:"language_options" json:"language_options"`
	BatchSize       int            `yaml:"batch_size" json:"batch_size"`
	Deepseek        DeepseekConfig `yaml:"deepseek" json:"deepseek"`
}

// LLMClient 抽象大模型调用，便于测试注入。
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// JobProcessor 描述清洗接口。
type JobProcessor interface {
	Process(ctx context.Context, raw model.RawJob) (Result, error)
}

// ResultOutcome 指示处理结果。
type ResultOutcome string

const (
	ResultAccepted ResultOutcome = "accepted"
	ResultRejected ResultOutcome = "rejected"
)

// Result 包含处理结果与输出。
type Result struct {
	Outcome ResultOutcome
	Job     *model.Job
	Reason  string
	Trace   datatypes.JSONMap
}

// Processor 组合 LLM 与规则实现 JobProcessor。
type Processor struct {
	cfg       Config
	llm       LLMClient
	tagLookup map[string]string
}

// New 创建 Processor。
func New(cfg Config, llm LLMClient) *Processor {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 20
	}
	tagLookup := make(map[string]string)
	for _, tag := range cfg.TagCandidates {
		if trimmed := strings.TrimSpace(tag); trimmed != "" {
			tagLookup[strings.ToLower(trimmed)] = trimmed
		}
	}
	return &Processor{cfg: cfg, llm: llm, tagLookup: tagLookup}
}

// Process 执行关键词初筛 + LLM 归一化。
func (p *Processor) Process(ctx context.Context, raw model.RawJob) (Result, error) {
	text := strings.TrimSpace(raw.Title + "\n" + raw.Summary + "\n" + raw.Content)
	if !p.containsKeyword(text) {
		return Result{Outcome: ResultRejected, Reason: "missing required keywords"}, nil
	}

	prompt := p.buildPrompt(raw, text)
	respText, err := p.llm.Complete(ctx, prompt)
	if err != nil {
		return Result{}, fmt.Errorf("llm complete: %w", err)
	}

	trace := datatypes.JSONMap{"prompt": prompt, "llm_response": respText}

	var payload llmClassification
	if err := json.Unmarshal([]byte(respText), &payload); err != nil {
		return Result{}, fmt.Errorf("parse llm response: %w", err)
	}

	if !payload.IsRemote {
		reason := payload.Verdict
		if reason == "" {
			reason = "llm rejected"
		}
		return Result{Outcome: ResultRejected, Reason: reason, Trace: trace}, nil
	}

	job := p.buildJob(raw, payload)
	return Result{Outcome: ResultAccepted, Job: &job, Trace: trace}, nil
}

func (p *Processor) containsKeyword(text string) bool {
	if len(p.cfg.Keywords) == 0 {
		return true
	}
	lower := strings.ToLower(text)
	for _, kw := range p.cfg.Keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func (p *Processor) buildPrompt(raw model.RawJob, text string) string {
	template := strings.TrimSpace(p.cfg.PromptTemplate)
	if template == "" {
		template = defaultPrompt
	}
	tagList := strings.Join(p.cfg.TagCandidates, ", ")
	prompt := strings.ReplaceAll(template, "{{TEXT}}", text)
	prompt = strings.ReplaceAll(prompt, "{{TAGS}}", tagList)

	instructions := `\n请严格输出 JSON，对象字段:{"is_remote":bool,"summary":string,"verdict":string,"employment_type":string,"salary_range":string,"role_category":string,"language_requirement":string,"score":int,"tags":string数组,"skill_tags":string数组}.`
	return prompt + instructions
}

func (p *Processor) buildJob(raw model.RawJob, payload llmClassification) model.Job {
	job := model.Job{
		ID:                  raw.ExternalID,
		Title:               strings.TrimSpace(raw.Title),
		Summary:             strings.TrimSpace(payload.Summary),
		Source:              raw.Source,
		URL:                 raw.URL,
		Tags:                raw.Tags,
		RawAttributes:       raw.RawPayload,
		PublishedAt:         raw.PublishedAt,
		EmploymentType:      payload.EmploymentType,
		SalaryRange:         payload.SalaryRange,
		RoleCategory:        payload.RoleCategory,
		LanguageRequirement: payload.LanguageRequirement,
		Score:               clampScore(payload.Score),
		Verdict:             payload.Verdict,
		NormalizedTags:      datatypes.JSONMap{},
		SkillTags:           datatypes.JSONMap{},
	}
	if job.ID == "" {
		job.ID = fmt.Sprintf("%s-%d", raw.Source, raw.ID)
	}
	if job.Summary == "" {
		job.Summary = raw.Summary
	}
	if job.RawAttributes == nil {
		job.RawAttributes = datatypes.JSONMap{}
	}
	if job.Tags == nil {
		job.Tags = datatypes.JSONMap{}
	}

	for _, tag := range payload.Tags {
		key := strings.ToLower(strings.TrimSpace(tag))
		if canonical, ok := p.tagLookup[key]; ok {
			job.NormalizedTags[canonical] = true
		}
	}
	if len(job.NormalizedTags) == 0 {
		job.NormalizedTags = datatypes.JSONMap{}
	}

	for _, tag := range payload.SkillTags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		job.SkillTags[trimmed] = true
	}

	return job
}

func clampScore(score int) int {
	if score < 0 {
		return 0
	}
	if score > 5 {
		return 5
	}
	return score
}

const defaultPrompt = `请作为资深 HR，阅读以下招聘文本并输出结构化判断：\n{{TEXT}}\n可选岗位标签: {{TAGS}}。需要判断岗位是否为远程，并对岗位进行简要总结与打标签。`

// llmClassification 对应 LLM JSON 响应。
type llmClassification struct {
	IsRemote            bool     `json:"is_remote"`
	Summary             string   `json:"summary"`
	Verdict             string   `json:"verdict"`
	EmploymentType      string   `json:"employment_type"`
	SalaryRange         string   `json:"salary_range"`
	RoleCategory        string   `json:"role_category"`
	LanguageRequirement string   `json:"language_requirement"`
	Score               int      `json:"score"`
	Tags                []string `json:"tags"`
	SkillTags           []string `json:"skill_tags"`
}
