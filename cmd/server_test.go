package main

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// 确保收到取消信号时会触发服务器优雅关闭。
func TestRunServer_ShutdownOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched := newStubCancelScheduler()
	srv := newStubServer()

	done := make(chan error, 1)
	go func() {
		done <- runServer(ctx, srv, sched, 500*time.Millisecond)
	}()

	srv.waitStarted(t)

	cancel()

	srv.waitShutdown(t)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServer returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runServer did not return after cancel")
	}

	if sched.canceled.Load() == 0 {
		t.Fatalf("scheduler did not observe context cancellation")
	}
}

type stubServer struct {
	started        chan struct{}
	shutdownCalled chan struct{}
	closed         atomic.Bool
}

func newStubServer() *stubServer {
	return &stubServer{
		started:        make(chan struct{}),
		shutdownCalled: make(chan struct{}),
	}
}

func (s *stubServer) ListenAndServe() error {
	close(s.started)
	<-s.shutdownCalled
	return http.ErrServerClosed
}

func (s *stubServer) Shutdown(context.Context) error {
	if s.closed.Swap(true) {
		return nil
	}
	close(s.shutdownCalled)
	return nil
}

func (s *stubServer) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-s.started:
	case <-time.After(time.Second):
		t.Fatal("server did not start")
	}
}

func (s *stubServer) waitShutdown(t *testing.T) {
	t.Helper()
	select {
	case <-s.shutdownCalled:
	case <-time.After(time.Second):
		t.Fatal("server shutdown was not called")
	}
}

type stubCancelScheduler struct {
	canceled atomic.Int32
}

func newStubCancelScheduler() *stubCancelScheduler {
	return &stubCancelScheduler{}
}

func (s *stubCancelScheduler) Start(ctx context.Context) error {
	<-ctx.Done()
	s.canceled.Add(1)
	return ctx.Err()
}

func (s *stubCancelScheduler) RunOnce(context.Context) (int, error) {
	return 0, nil
}
