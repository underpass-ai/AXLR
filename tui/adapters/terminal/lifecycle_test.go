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

// List waits for cancellation so shutdown competes with publishing completion.
type cancellingListStore struct {
	application.SessionStorePort
	started chan struct{}
}

func (s cancellingListStore) List(ctx context.Context) ([]domain.SessionSummary, error) {
	close(s.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestNavigationSessionListShutdownDoesNotPanicOrBlock(t *testing.T) {
	// Exercise both ready select branches: completion may arrive or be discarded
	// because the terminal has gone away. Neither result may panic the wrapper.
	for i := 0; i < 64; i++ {
		started := make(chan struct{})
		original := New(Dependencies{Store: cancellingListStore{started: started}})
		_, cmd := original.Update(ControlIntent("sessions"))
		result := make(chan any, 1)
		go func() { defer func() { result <- recover() }(); cmd() }()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("list did not start")
		}
		closed := make(chan struct{})
		go func() { original.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("shutdown blocked")
		}
		select {
		case panicValue := <-result:
			if panicValue != nil {
				t.Fatalf("session-list shutdown panicked: %v", panicValue)
			}
		case <-time.After(time.Second):
			t.Fatal("session-list command blocked")
		}
	}
}
