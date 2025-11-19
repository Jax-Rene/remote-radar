package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"remote-radar/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store 封装 SQLite 数据库访问
// 负责初始化、迁移以及职位数据的增删查操作。
type Store struct {
	db *gorm.DB
}

// UpsertResult 表示写入结果。
type UpsertResult struct {
	Created int
	NewJobs []model.Job
}

// NewStore 创建一个使用 SQLite 的 Store 并自动迁移表结构。
func NewStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.AutoMigrate(&model.Job{}); err != nil {
		return nil, fmt.Errorf("auto migrate Job: %w", err)
	}

	return &Store{db: db}, nil
}

// Close 关闭底层数据库连接。
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("get sql DB: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close db: %w", err)
	}
	return nil
}

// UpsertJobs 写入职位列表，已有记录按主键覆盖更新。
// 返回新增的记录数与新增记录。
func (s *Store) UpsertJobs(ctx context.Context, jobs []model.Job) (UpsertResult, error) {
	res := UpsertResult{}
	if len(jobs) == 0 {
		return res, nil
	}

	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}

	// 先获取已存在的 ID，便于统计新增数量。
	var existing []string
	if err := s.db.WithContext(ctx).Model(&model.Job{}).Where("id IN ?", ids).Pluck("id", &existing).Error; err != nil {
		return res, fmt.Errorf("query existing ids: %w", err)
	}

	existingSet := make(map[string]struct{}, len(existing))
	for _, id := range existing {
		existingSet[id] = struct{}{}
	}

	for i, id := range ids {
		if _, ok := existingSet[id]; !ok {
			res.Created++
			res.NewJobs = append(res.NewJobs, jobs[i])
			existingSet[id] = struct{}{} // 防止输入中重复 ID 被重复计数
		}
	}

	tx := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "summary", "published_at", "source", "url", "tags", "raw_attributes", "updated_at"}),
	}).Create(&jobs)
	if tx.Error != nil {
		return res, fmt.Errorf("upsert jobs: %w", tx.Error)
	}

	return res, nil
}

// ListJobs 返回按发布时间倒序的职位列表，支持 limit 与 offset。
func (s *Store) ListJobs(ctx context.Context, limit, offset int) ([]model.Job, error) {
	var jobs []model.Job
	if offset < 0 {
		offset = 0
	}
	query := s.db.WithContext(ctx).Order("published_at DESC")
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return jobs, nil
}

// CountJobs 返回职位总数。
func (s *Store) CountJobs(ctx context.Context) (int64, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&model.Job{}).Count(&total).Error; err != nil {
		return 0, fmt.Errorf("count jobs: %w", err)
	}
	return total, nil
}

// GetJob 根据 ID 获取职位。
func (s *Store) GetJob(ctx context.Context, id string) (*model.Job, error) {
	var job model.Job
	if err := s.db.WithContext(ctx).First(&job, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("get job: %w", err)
	}
	return &job, nil
}
