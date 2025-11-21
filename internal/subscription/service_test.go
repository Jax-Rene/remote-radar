package subscription

import (
	"context"
	"errors"
	"testing"

	"remote-radar/internal/model"
)

func TestServiceValidatesAndCreatesSubscription(t *testing.T) {
	t.Parallel()

	store := &stubStore{}
	svc := NewService(store, Config{AllowedChannels: []string{"email"}, TagCandidates: []string{"backend", "frontend"}})

	req := Request{Email: "user@example.com", Channel: "email", Tags: []string{"backend"}}
	sub, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if store.calls != 1 {
		t.Fatalf("expected store Create called once, got %d", store.calls)
	}
	if sub.Email != req.Email || sub.Channel != req.Channel {
		t.Fatalf("unexpected subscription returned: %+v", sub)
	}
}

func TestServiceRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	store := &stubStore{}
	svc := NewService(store, Config{AllowedChannels: []string{"email"}, TagCandidates: []string{"backend"}})

	cases := []Request{
		{Email: "", Channel: "email"},
		{Email: "bad", Channel: "email"},
		{Email: "user@example.com", Channel: "sms"},
		{Email: "user@example.com", Channel: "email", Tags: []string{"unknown"}},
	}
	for i, req := range cases {
		if _, err := svc.Create(context.Background(), req); err == nil {
			t.Fatalf("case %d expected error", i)
		}
	}
	if store.calls != 0 {
		t.Fatalf("expected store not called on invalid input")
	}
}

func TestServicePropagatesStoreError(t *testing.T) {
	t.Parallel()

	store := &stubStore{err: errors.New("boom")}
	svc := NewService(store, Config{AllowedChannels: []string{"email"}, TagCandidates: []string{"backend"}})

	_, err := svc.Create(context.Background(), Request{Email: "user@example.com", Channel: "email"})
	if err == nil {
		t.Fatalf("expected error when store fails")
	}
}

type stubStore struct {
	calls int
	err   error
}

func (s *stubStore) CreateSubscription(ctx context.Context, sub *model.Subscription) error {
	s.calls++
	if s.err != nil {
		return s.err
	}
	sub.ID = 1
	return nil
}
