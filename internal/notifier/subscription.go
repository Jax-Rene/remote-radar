package notifier

import (
	"context"
	"fmt"
	"strings"

	"remote-radar/internal/model"
)

// SubscriptionStore 定义订阅读取接口。
type SubscriptionStore interface {
	ListSubscriptions(ctx context.Context) ([]model.Subscription, error)
}

// jobNotifier 提供统一通知接口。
type jobNotifier interface {
	Notify(ctx context.Context, jobs []model.Job) error
}

// SubscriptionNotifier 会按订阅偏好推送通知。
type SubscriptionNotifier struct {
	store    SubscriptionStore
	emailCfg EmailConfig
	sender   EmailSender
	fallback jobNotifier
}

// NewSubscriptionNotifier 创建实例。
func NewSubscriptionNotifier(store SubscriptionStore, cfg EmailConfig, sender EmailSender, fallback jobNotifier) *SubscriptionNotifier {
	return &SubscriptionNotifier{
		store:    store,
		emailCfg: cfg,
		sender:   sender,
		fallback: fallback,
	}
}

// Notify 根据订阅过滤并发送消息。
func (n *SubscriptionNotifier) Notify(ctx context.Context, jobs []model.Job) error {
	if len(jobs) == 0 || n.store == nil {
		return nil
	}

	subs, err := n.store.ListSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("list subscriptions: %w", err)
	}
	if len(subs) == 0 {
		if n.fallback != nil {
			return n.fallback.Notify(ctx, jobs)
		}
		return nil
	}

	for _, sub := range subs {
		matches := filterJobsBySubscription(sub, jobs)
		if len(matches) == 0 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(sub.Channel)) {
		case "email", "":
			cfg := n.emailCfg
			cfg.To = []string{sub.Email}
			email := NewEmailNotifier(cfg, n.sender)
			if err := email.Notify(ctx, matches); err != nil {
				return err
			}
		default:
			continue
		}
	}

	return nil
}

func filterJobsBySubscription(sub model.Subscription, jobs []model.Job) []model.Job {
	if len(sub.Tags) == 0 {
		return jobs
	}
	filtered := make([]model.Job, 0, len(jobs))
	for _, job := range jobs {
		if jobMatches(job, sub.Tags) {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func jobMatches(job model.Job, tags map[string]any) bool {
	if len(tags) == 0 {
		return true
	}
	if job.NormalizedTags == nil {
		return false
	}
	for k, v := range tags {
		if !isTruthy(v) {
			continue
		}
		if !isTruthy(job.NormalizedTags[k]) {
			return false
		}
	}
	return true
}

func isTruthy(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.TrimSpace(strings.ToLower(val)) == "true"
	case float64:
		return val != 0
	default:
		return val != nil
	}
}
