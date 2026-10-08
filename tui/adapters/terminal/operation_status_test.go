package terminal

import (
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestNewOperationClearsStaleStatusError(t *testing.T) {
	t.Run("continue", func(t *testing.T) {
		m := sized()
		m.Header.State.Status = domain.StatusInterrupted
		m.Status.Error = "invalid JSON value"
		m = update(m, ControlIntent("continue"))
		if !m.Busy {
			t.Fatal("continue did not start an operation")
		}
		if m.Status.Error != "" {
			t.Fatalf("continue kept the previous operation's error: %q", m.Status.Error)
		}
	})
	t.Run("queued prompt", func(t *testing.T) {
		m := sized()
		m.Busy = true
		m.Header.State.Status = domain.StatusStreaming
		m.Status.Error = "invalid JSON value"
		m.Composer.Input.SetValue("queued prompt")
		m = update(m, ControlIntent("send"))
		if m.Status.Error != "" {
			t.Fatalf("queued prompt kept the previous operation's error: %q", m.Status.Error)
		}
	})
}
