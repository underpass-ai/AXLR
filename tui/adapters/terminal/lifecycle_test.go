package terminal

import (
	"context"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"testing"
	"time"
)

func TestAppModelCloseCancelsAndWaitsAcrossCopies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	original := New(Dependencies{Context: ctx})
	copy := original
	started := make(chan struct{})
	stopped := make(chan struct{})
	cmd := copy.BeginOperation(func(ctx context.Context, _ *domain.Session, emit func(application.Event) error) error {
		close(started)
		for i := 0; i < 100; i++ {
			if err := emit(application.Event{}); err != nil {
				break
			}
		}
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	go cmd()
	<-started
	done := make(chan struct{})
	go func() { original.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked on undrained event queue")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("closed before operation stopped")
	}
	original.Close()
}
