package main

import (
	"context"
	"errors"
	"testing"
)

func TestRunOnceManual(t *testing.T) {
	t.Parallel()

	stub := &stubScheduler{created: 3}
	builds := 0

	created, err := runOnceManual(context.Background(), AppConfig{}, func(AppConfig) (appDeps, func(), error) {
		builds++
		return appDeps{sched: stub}, func() {}, nil
	})
	if err != nil {
		t.Fatalf("runOnceManual error: %v", err)
	}
	if created != 3 {
		t.Fatalf("expected created=3, got %d", created)
	}
	if builds != 1 {
		t.Fatalf("expected builder called once, got %d", builds)
	}
	if stub.runOnceCalls != 1 {
		t.Fatalf("expected RunOnce called once, got %d", stub.runOnceCalls)
	}
}

func TestRunOnceManualBuilderError(t *testing.T) {
	t.Parallel()

	_, err := runOnceManual(context.Background(), AppConfig{}, func(AppConfig) (appDeps, func(), error) {
		return appDeps{}, func() {}, errors.New("build fail")
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

// --- stubs ---

type stubScheduler struct {
	created      int
	runOnceCalls int
}

func (s *stubScheduler) RunOnce(context.Context) (int, error) {
	s.runOnceCalls++
	return s.created, nil
}

func (s *stubScheduler) Start(context.Context) error {
	return nil
}
