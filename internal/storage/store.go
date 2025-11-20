package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"remote-radar/internal/model"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store 封装 SQLite 数据库访问，负责职位、原始数据、订阅的增删查。
type Store struct {
	db *gorm.DB
}

// UpsertResult 表示最终职位写入结果。
type UpsertResult struct {
	Created int
	NewJobs []model.Job
}

// RawUpsertResult 表示原始抓取数据写入结果。
type RawUpsertResult struct {
	Created int
	NewJobs []model.RawJob
}

// JobQueryOptions 提供职位查询过滤条件。
type JobQueryOptions struct {
	Limit  int
	Offset int
	Tags   []string
}

// RawJobQuery 描述原始数据筛选条件。
type RawJobQuery struct {
	Status model.RawJobStatus
	Limit  int
}

// RawJobStatusUpdate 用于更新原始数据状态。
type RawJobStatusUpdate struct {
	Status  model.RawJobStatus
	Reason  string
	Details datatypes.JSONMap
}

// NewStore 创建 Store 并自动迁移数据表。
func NewStore(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.AutoMigrate(&model.Job{}, &model.RawJob{}, &model.Subscription{}); err != nil {
		return nil, fmt.Errorf("auto migrate models: %w", err)
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

// UpsertJobs 写入职位列表，已有主键则更新，返回新增数量与新增记录。
func (s *Store) UpsertJobs(ctx context.Context, jobs []model.Job) (UpsertResult, error) {
	res := UpsertResult{}
	if len(jobs) == 0 {
		return res, nil
	}

	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}

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
			existingSet[id] = struct{}{}
		}
	}

	tx := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"title",
			"summary",
			"published_at",
			"source",
			"url",
			"tags",
			"raw_attributes",
			"normalized_tags",
			"skill_tags",
			"employment_type",
			"salary_range",
			"role_category",
			"language_requirement",
			"score",
			"verdict",
			"updated_at",
		}),
	}).Create(&jobs)
	if tx.Error != nil {
		return res, fmt.Errorf("upsert jobs: %w", tx.Error)
	}

	return res, nil
}

// UpsertRawJobs 写入原始抓取数据，按 source + external_id 去重。
func (s *Store) UpsertRawJobs(ctx context.Context, jobs []model.RawJob) (RawUpsertResult, error) {
	res := RawUpsertResult{}
	if len(jobs) == 0 {
		return res, nil
	}

	bySource := make(map[string][]string)
	for i := range jobs {
		if jobs[i].Status == "" {
			jobs[i].Status = model.RawJobStatusPending
		}
		bySource[jobs[i].Source] = append(bySource[jobs[i].Source], jobs[i].ExternalID)
	}

	existing := make(map[string]struct{})
	for source, ids := range bySource {
		if len(ids) == 0 {
			continue
		}
		var rows []string
		if err := s.db.WithContext(ctx).Model(&model.RawJob{}).
			Where("source = ? AND external_id IN ?", source, ids).
			Pluck("external_id", &rows).Error; err != nil {
			return res, fmt.Errorf("query existing raw ids: %w", err)
		}
		for _, ext := range rows {
			existing[source+"|"+ext] = struct{}{}
		}
	}

	for i := range jobs {
		key := jobs[i].Source + "|" + jobs[i].ExternalID
		if _, ok := existing[key]; !ok {
			res.Created++
			res.NewJobs = append(res.NewJobs, jobs[i])
			existing[key] = struct{}{}
		}
	}

	tx := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "source"}, {Name: "external_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "summary", "content", "url", "tags", "raw_payload", "published_at", "updated_at"}),
	}).Create(&jobs)
	if tx.Error != nil {
		return res, fmt.Errorf("upsert raw jobs: %w", tx.Error)
	}

	return res, nil
}

// ListJobs 返回按发布时间倒序的职位列表。
func (s *Store) ListJobs(ctx context.Context, opts JobQueryOptions) ([]model.Job, error) {
	var jobs []model.Job
	if opts.Offset < 0 {
		opts.Offset = 0
	}

	query := s.db.WithContext(ctx).Model(&model.Job{}).Order("published_at DESC")
	query = applyJobFilters(query, opts)
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}

	if err := query.Find(&jobs).Error; err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	return jobs, nil
}

// CountJobs 返回满足过滤条件的职位数量。
func (s *Store) CountJobs(ctx context.Context, opts JobQueryOptions) (int64, error) {
	var total int64
	query := applyJobFilters(s.db.WithContext(ctx).Model(&model.Job{}), opts)
	if err := query.Count(&total).Error; err != nil {
		return 0, fmt.Errorf("count jobs: %w", err)
	}
	return total, nil
}

// ListRawJobs 返回指定状态的原始数据，默认 pending，按创建时间升序。
func (s *Store) ListRawJobs(ctx context.Context, query RawJobQuery) ([]model.RawJob, error) {
	var raws []model.RawJob
	status := query.Status
	if status == "" {
		status = model.RawJobStatusPending
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if err := s.db.WithContext(ctx).
		Where("status = ?", status).
		Order("created_at ASC").
		Limit(limit).
		Find(&raws).Error; err != nil {
		return nil, fmt.Errorf("list raw jobs: %w", err)
	}
	return raws, nil
}

// UpdateRawJobStatus 更新原始数据状态及 LLM 详情。
func (s *Store) UpdateRawJobStatus(ctx context.Context, id uint, update RawJobStatusUpdate) error {
	if update.Status == "" {
		update.Status = model.RawJobStatusProcessed
	}
	values := map[string]any{
		"status": update.Status,
		"reason": update.Reason,
	}
	if update.Details != nil {
		values["llm_response"] = update.Details
	}
	tx := s.db.WithContext(ctx).Model(&model.RawJob{}).Where("id = ?", id).Updates(values)
	if tx.Error != nil {
		return fmt.Errorf("update raw job status: %w", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return fmt.Errorf("update raw job status: id %d not found", id)
	}
	return nil
}

// CreateSubscription 新增订阅。
func (s *Store) CreateSubscription(ctx context.Context, sub *model.Subscription) error {
	if err := s.db.WithContext(ctx).Create(sub).Error; err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}
	return nil
}

// ListSubscriptions 返回所有订阅记录。
func (s *Store) ListSubscriptions(ctx context.Context) ([]model.Subscription, error) {
	var subs []model.Subscription
	if err := s.db.WithContext(ctx).Order("created_at ASC").Find(&subs).Error; err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return subs, nil
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

func applyJobFilters(db *gorm.DB, opts JobQueryOptions) *gorm.DB {
	if len(opts.Tags) == 0 {
		return db
	}
	for _, tag := range opts.Tags {
		if tag == "" {
			continue
		}
		path := fmt.Sprintf("$.\"%s\"", tag)
		db = db.Where("json_extract(normalized_tags, ?) = 1", path)
	}
	return db
}
