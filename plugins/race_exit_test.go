package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/domain"
)

// quietRaceExit goes into every helper child's environment. A test binary
// built with -race sleeps GORACE's atexit_sleep_ms (1000 ms by default)
// before it exits, exactly mcpclient's TerminateDuration: every close then
// waited the full second and raced the SIGTERM against the child's own exit.
const quietRaceExit = "GORACE=atexit_sleep_ms=0"

// Closing a helper plugin must not wait for the transport's terminate
// timer: under -race it did, on every close, and the kill that followed
// sometimes found the child already gone.
func TestManagerCloseIsNotHeldByTheChildExit(t *testing.T) {
	m, err := NewManager([]Registration{pluginRegistration(t, "quick", []domain.PluginToolName{"echo"})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed >= 500*time.Millisecond {
		t.Fatalf("close took %v: the child's exit reached the transport's 1 s terminate timer", elapsed)
	}
}
