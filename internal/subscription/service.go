package subscription

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"remote-radar/internal/model"

	"gorm.io/datatypes"
)

// Store 定义持久化接口。
type Store interface {
	CreateSubscription(ctx context.Context, sub *model.Subscription) error
}

// Config 控制可用渠道与可选标签。
type Config struct {
	AllowedChannels []string `yaml:"allowed_channels" json:"allowed_channels"`
	TagCandidates   []string `yaml:"tag_candidates" json:"tag_candidates"`
}

// Request 表示前端订阅请求。
type Request struct {
	Email   string   `json:"email"`
	Channel string   `json:"channel"`
	Tags    []string `json:"tags"`
}

// Service 负责验证与写入订阅偏好。
type Service struct {
	store    Store
	channels map[string]struct{}
	tags     map[string]string
}

// NewService 创建订阅服务。
func NewService(store Store, cfg Config) *Service {
	channelMap := make(map[string]struct{})
	for _, ch := range cfg.AllowedChannels {
		if trimmed := strings.ToLower(strings.TrimSpace(ch)); trimmed != "" {
			channelMap[trimmed] = struct{}{}
		}
	}
	if len(channelMap) == 0 {
		channelMap["email"] = struct{}{}
	}
	tagLookup := make(map[string]string)
	for _, tag := range cfg.TagCandidates {
		if trimmed := strings.TrimSpace(tag); trimmed != "" {
			tagLookup[strings.ToLower(trimmed)] = trimmed
		}
	}
	return &Service{store: store, channels: channelMap, tags: tagLookup}
}

// Create 校验请求并写入数据库。
func (s *Service) Create(ctx context.Context, req Request) (model.Subscription, error) {
	email := strings.TrimSpace(req.Email)
	if email == "" {
		return model.Subscription{}, fmt.Errorf("email required")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return model.Subscription{}, fmt.Errorf("invalid email: %w", err)
	}

	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if channel == "" {
		channel = "email"
	}
	if _, ok := s.channels[channel]; !ok {
		return model.Subscription{}, fmt.Errorf("unsupported channel %s", channel)
	}

	tagMap := datatypes.JSONMap{}
	for _, tag := range req.Tags {
		key := strings.ToLower(strings.TrimSpace(tag))
		if key == "" {
			continue
		}
		canonical, ok := s.tags[key]
		if !ok && len(s.tags) > 0 {
			return model.Subscription{}, fmt.Errorf("unknown tag %s", tag)
		}
		if canonical == "" {
			canonical = strings.TrimSpace(tag)
		}
		tagMap[canonical] = true
	}

	sub := model.Subscription{
		Email:   email,
		Channel: channel,
		Tags:    tagMap,
	}
	if err := s.store.CreateSubscription(ctx, &sub); err != nil {
		return model.Subscription{}, err
	}
	return sub, nil
}
