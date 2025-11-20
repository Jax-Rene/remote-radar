package notifier

import (
	"context"
	"strings"
	"testing"

	"remote-radar/internal/model"

	"gorm.io/datatypes"
)

func TestSubscriptionNotifierFiltersJobsPerSubscriber(t *testing.T) {
	t.Parallel()

	store := &stubSubscriptionStore{
		subs: []model.Subscription{
			{ID: 1, Email: "be@example.com", Channel: "email", Tags: datatypes.JSONMap{"backend": true}},
			{ID: 2, Email: "log@example.com", Channel: "log", Tags: datatypes.JSONMap{"frontend": true}},
		},
	}

	emailSender := &stubSender{}
	logStub := &stubJobNotifier{}
	cfg := EmailConfig{From: "from@example.com", Host: "smtp", To: []string{"placeholder"}}
	subNotifier := NewSubscriptionNotifier(store, cfg, emailSender, logStub)

	jobs := []model.Job{
		{ID: "be", Title: "Backend", NormalizedTags: datatypes.JSONMap{"backend": true}},
		{ID: "fe", Title: "Frontend", NormalizedTags: datatypes.JSONMap{"frontend": true}},
	}

	if err := subNotifier.Notify(context.Background(), jobs); err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	if emailSender.calls != 1 {
		t.Fatalf("expected email sender called once, got %d", emailSender.calls)
	}
	if !strings.Contains(emailSender.lastBody, "Backend") {
		t.Fatalf("expected backend job in email body, got %s", emailSender.lastBody)
	}
	if logStub.calls != 1 || len(logStub.last) != 1 || logStub.last[0].ID != "fe" {
		t.Fatalf("expected log notifier to receive frontend job, got %+v", logStub.last)
	}
}

func TestSubscriptionNotifierFallsBackWhenNoSubscriptions(t *testing.T) {
	t.Parallel()

	store := &stubSubscriptionStore{}
	fallback := &stubNotifier{}

	notifier := NewSubscriptionNotifier(store, EmailConfig{}, nil, fallback)

	jobs := []model.Job{{ID: "only"}}

	if err := notifier.Notify(context.Background(), jobs); err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	if fallback.calls == 0 {
		t.Fatalf("expected fallback notifier to be invoked")
	}
}

type stubSubscriptionStore struct {
	subs []model.Subscription
}

func (s *stubSubscriptionStore) ListSubscriptions(ctx context.Context) ([]model.Subscription, error) {
	return s.subs, nil
}

type stubNotifier struct {
	calls int
}

func (s *stubNotifier) Notify(ctx context.Context, jobs []model.Job) error {
	s.calls++
	return nil
}

type stubJobNotifier struct {
	calls int
	last  []model.Job
}

func (s *stubJobNotifier) Notify(ctx context.Context, jobs []model.Job) error {
	s.calls++
	s.last = append([]model.Job(nil), jobs...)
	return nil
}
