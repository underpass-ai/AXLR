package terminal

import (
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 14 October 2026 the footer kept the last turn's "complete" beside
// "esc cancel" after a prompt was sent, until the first stream event.
func TestFooterSaysSendingUntilTheTurnReportsItsState(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m = update(m, application.Event{Kind: application.EventState, State: domain.StatusComplete})
	m.Composer.Input.SetValue("hola")
	m = update(m, ControlIntent("send"))
	footer := m.footerView()
	if !m.Busy || !strings.Contains(footer, m.Theme.T("status.sending")) || strings.Contains(footer, m.Theme.T("status.complete")) {
		t.Fatalf("a sent prompt still reads as the last turn: %q", footer)
	}
	m = update(m, application.Event{Kind: application.EventState, State: domain.StatusStreaming})
	if strings.Contains(m.footerView(), m.Theme.T("status.sending")) {
		t.Fatal("sending outlived the turn's state")
	}
}
