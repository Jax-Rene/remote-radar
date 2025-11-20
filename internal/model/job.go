package model

import (
	"time"

	"gorm.io/datatypes"
)

// Job 表示清洗后的最终职位数据
// - NormalizedTags/SkillTags: LLM 归一化标签集合
// - EmploymentType 等字段为结构化结果
// - Score/Verdict: HR 评估得分与结论
// - RawAttributes: 保留原始明细方便回溯
// - CreatedAt/UpdatedAt: 由 GORM 自动维护
// 中文注释便于快速了解字段用途。
type Job struct {
	ID                  string            `gorm:"primaryKey" json:"id"`
	Title               string            `json:"title"`
	Summary             string            `json:"summary"`
	PublishedAt         time.Time         `json:"published_at"`
	Source              string            `json:"source"`
	URL                 string            `json:"url"`
	Tags                datatypes.JSONMap `json:"tags"`
	RawAttributes       datatypes.JSONMap `json:"raw_attributes"`
	NormalizedTags      datatypes.JSONMap `json:"normalized_tags"`
	SkillTags           datatypes.JSONMap `json:"skill_tags"`
	EmploymentType      string            `json:"employment_type"`
	SalaryRange         string            `json:"salary_range"`
	RoleCategory        string            `json:"role_category"`
	LanguageRequirement string            `json:"language_requirement"`
	Score               int               `json:"score"`
	Verdict             string            `json:"verdict"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

// RawJobStatus 描述原始数据的处理状态。
type RawJobStatus string

const (
	RawJobStatusPending   RawJobStatus = "pending"
	RawJobStatusProcessed RawJobStatus = "processed"
	RawJobStatusRejected  RawJobStatus = "rejected"
)

// RawJob 存储抓取的原始职位内容，支持重新清洗回溯。
type RawJob struct {
	ID          uint              `gorm:"primaryKey" json:"id"`
	Source      string            `gorm:"uniqueIndex:idx_raw_source_external" json:"source"`
	ExternalID  string            `gorm:"uniqueIndex:idx_raw_source_external" json:"external_id"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	Content     string            `json:"content"`
	URL         string            `json:"url"`
	Tags        datatypes.JSONMap `json:"tags"`
	RawPayload  datatypes.JSONMap `json:"raw_payload"`
	PublishedAt time.Time         `json:"published_at"`
	Status      RawJobStatus      `json:"status"`
	Reason      string            `json:"reason"`
	LLMResponse datatypes.JSONMap `json:"llm_response"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Subscription 记录用户订阅偏好。
type Subscription struct {
	ID        uint              `gorm:"primaryKey" json:"id"`
	Email     string            `json:"email"`
	Channel   string            `json:"channel"`
	Tags      datatypes.JSONMap `json:"tags"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}
