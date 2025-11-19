package notifier

import (
	"context"
	"log"
	"strings"
	"testing"

	"remote-radar/internal/model"
)

func TestLogNotifierWritesJobs(t *testing.T) {
	var buf strings.Builder
	logger := log.New(&buf, "", 0)
	n := NewLogNotifier(logger)

	jobs := []model.Job{{
		Title:  "Test Role",
		Source: "eleduck",
		URL:    "https://example.com/1",
	}}

	if err := n.Notify(context.Background(), jobs); err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	logged := buf.String()
	if !strings.Contains(logged, "Test Role") || !strings.Contains(logged, "https://example.com/1") {
		t.Fatalf("log output missing job info: %s", logged)
	}
}

func TestLogNotifierSkipsEmptyJobs(t *testing.T) {
	var buf strings.Builder
	logger := log.New(&buf, "", 0)
	n := NewLogNotifier(logger)

	if err := n.Notify(context.Background(), nil); err != nil {
		t.Fatalf("Notify error: %v", err)
	}

	if buf.Len() != 0 {
		t.Fatalf("expected no log output, got %q", buf.String())
	}
}
