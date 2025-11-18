package model

import (
	"time"

	"gorm.io/datatypes"
)

// Job 表示一个远程职位
// 中文注释说明字段用途
// - ID: 平台唯一标识
// - Tags: 存储平台标签，键值对
// - PublishedAt: 发布时间，用于排序与增量抓取
// - Source/URL: 数据来源信息
// - CreatedAt/UpdatedAt: 由 GORM 自动维护

type Job struct {
	ID          string            `gorm:"primaryKey" json:"id"`
	Title       string            `json:"title"`
	Summary     string            `json:"summary"`
	PublishedAt time.Time         `json:"published_at"`
	Source      string            `json:"source"`
	URL         string            `json:"url"`
	Tags        datatypes.JSONMap `json:"tags"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}
