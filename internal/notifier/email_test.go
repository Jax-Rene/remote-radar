package notifier

import (
	"context"
	"strings"
	"testing"

	"remote-radar/internal/model"
)

func TestEmailNotifierSendsWhenNewJobs(t *testing.T) {
	t.Parallel()

	sender := &stubSender{}
	n := EmailNotifier{cfg: EmailConfig{From: "from@example.com", To: []string{"to@example.com"}}, sender: sender}

	jobs := []model.Job{{ID: "1", Title: "Remote"}}
	err := n.Notify(context.Background(), jobs)
	if err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected 1 send call, got %d", sender.calls)
	}
	if !strings.Contains(sender.lastBody, "Remote") {
		t.Fatalf("expected body to contain job title, got %s", sender.lastBody)
	}
}

func TestEmailNotifierSkipsWhenEmpty(t *testing.T) {
	t.Parallel()

	sender := &stubSender{}
	n := EmailNotifier{cfg: EmailConfig{From: "from@example.com", To: []string{"to@example.com"}}, sender: sender}

	if err := n.Notify(context.Background(), nil); err != nil {
		t.Fatalf("Notify error: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("expected no send calls, got %d", sender.calls)
	}
}

// --- stubs ---

type stubSender struct {
	calls    int
	lastBody string
	err      error
}

func (s *stubSender) Send(ctx context.Context, msg EmailMessage) error {
	s.calls++
	s.lastBody = msg.Body
	if s.err != nil {
		return s.err
	}
	return ctx.Err()
}
