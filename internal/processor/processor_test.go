package processor

import (
	"context"
	"strings"
	"testing"
	"time"

	"remote-radar/internal/model"

	"gorm.io/datatypes"
)

func TestProcessorRejectsWhenMissingKeywords(t *testing.T) {
	t.Parallel()

	cfg := Config{Keywords: []string{"远程", "remote"}, PromptTemplate: "classify {{TEXT}}"}
	llm := &stubLLM{}
	p := New(cfg, llm)

	raw := model.RawJob{
		Source:      "eleduck",
		ExternalID:  "raw-miss",
		Title:       "Onsite Dev",
		Summary:     "办公室办公",
		PublishedAt: time.Now(),
	}

	res, err := p.Process(context.Background(), raw)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if res.Outcome != ResultRejected {
		t.Fatalf("expected rejection, got %v", res.Outcome)
	}
	if llm.calls != 0 {
		t.Fatalf("expected no llm call when rejecting early, got %d", llm.calls)
	}
	if res.Reason == "" {
		t.Fatalf("expected rejection reason to be set")
	}
}

func TestProcessorAcceptsLLMResponse(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Keywords:        []string{"远程", "remote"},
		PromptTemplate:  "classify job: {{TEXT}} tags: {{TAGS}}",
		TagCandidates:   []string{"backend", "frontend", "fullstack"},
		EmploymentTypes: []string{"全职", "兼职"},
		SalaryRanges:    []string{"<$1000", "$1000~$5000", "$5000~$10000", ">$10000"},
		RoleCategories:  []string{"后端开发工程师", "前端开发工程师", "全栈开发工程师", "其他"},
		LanguageOptions: []string{"英语", "日语", "无"},
	}

	llm := &stubLLM{
		response: `{"is_remote":true,"summary":"Normalized summary","verdict":"远程可信","employment_type":"全职","salary_range":"$1000~$5000","role_category":"后端开发工程师","language_requirement":"英语","score":5,"tags":["backend","unknown"],"skill_tags":["Go","gRPC"]}`,
	}
	p := New(cfg, llm)

	raw := model.RawJob{
		Source:      "eleduck",
		ExternalID:  "ext-1",
		Title:       "Remote Go Dev",
		Summary:     "我们需要 remote Go 工程师",
		URL:         "https://example.com/job",
		Tags:        datatypes.JSONMap{"远程": true},
		RawPayload:  datatypes.JSONMap{"id": "ext-1"},
		PublishedAt: time.Now(),
	}

	res, err := p.Process(context.Background(), raw)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if res.Outcome != ResultAccepted {
		t.Fatalf("expected accepted outcome, got %v", res.Outcome)
	}
	if res.Job == nil {
		t.Fatalf("expected job returned")
	}
	job := *res.Job
	if job.ID != raw.ExternalID {
		t.Fatalf("expected job ID %s, got %s", raw.ExternalID, job.ID)
	}
	if job.Summary != "Normalized summary" {
		t.Fatalf("expected summary replaced by LLM, got %s", job.Summary)
	}
	if job.EmploymentType != "全职" {
		t.Fatalf("expected employment type propagated, got %s", job.EmploymentType)
	}
	if job.SalaryRange != "$1000~$5000" {
		t.Fatalf("expected salary range set, got %s", job.SalaryRange)
	}
	if job.LanguageRequirement != "英语" {
		t.Fatalf("expected language requirement, got %s", job.LanguageRequirement)
	}
	if job.Score != 5 {
		t.Fatalf("expected score 5, got %d", job.Score)
	}
	if job.NormalizedTags["backend"] != true {
		t.Fatalf("expected normalized backend tag, got %#v", job.NormalizedTags)
	}
	if job.NormalizedTags["unknown"] == true {
		t.Fatalf("expected unknown tags to be filtered out, got %#v", job.NormalizedTags)
	}
	if job.SkillTags["Go"] != true || job.SkillTags["gRPC"] != true {
		t.Fatalf("expected skill tags stored, got %#v", job.SkillTags)
	}
	if llm.lastPrompt == "" || !containsAll(llm.lastPrompt, []string{"Remote Go Dev", "backend"}) {
		t.Fatalf("expected prompt to include job text and tag candidates, got %q", llm.lastPrompt)
	}
	if len(res.Trace) == 0 {
		t.Fatalf("expected trace metadata to be recorded")
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}

type stubLLM struct {
	response   string
	err        error
	calls      int
	lastPrompt string
}

func (s *stubLLM) Complete(ctx context.Context, prompt string) (string, error) {
	s.calls++
	s.lastPrompt = prompt
	if s.err != nil {
		return "", s.err
	}
	return s.response, nil
}
