package terminal

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 10 October 2026 Esc during a turn left "! context canceled" in the
// footer, in place of the state and the session cost, and nothing in the
// conversation said the request had been stopped.
func TestEscapeShowsACancelledTurnInTheConversationAndKeepsTheFooter(t *testing.T) {
	for _, locale := range []Locale{English, Spanish} {
		m := sized()
		m.Theme.Locale = locale
		cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, _ func(application.Event) error) error {
			if err := s.BeginTurn("read the repository", nil); err != nil {
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		})
		m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
		m = update(m, cmd())
		view := ansi.Strip(m.View().Content)
		if m.Status.Error != "" || strings.Contains(view, context.Canceled.Error()) {
			t.Fatalf("%s: the raw error is shown: %q\n%s", locale, m.Status.Error, view)
		}
		footer := ansi.Strip(m.hintRow())
		if !strings.Contains(footer, m.Theme.T("status.cancelled")) || !strings.Contains(footer, m.Theme.T("footer.help")) {
			t.Fatalf("%s: the footer lost its state or hints: %q", locale, footer)
		}
		if !strings.Contains(m.Transcript.Text(), m.Theme.T("transcript.cancelled")) {
			t.Fatalf("%s: the conversation does not mark the cancelled request:\n%s", locale, m.Transcript.Text())
		}
		m.Header.State.Messages = append(m.Header.State.Messages, root.Message{Role: root.RoleUser, Content: "next"})
		m.refreshTranscript()
		if strings.Contains(m.Transcript.Text(), m.Theme.T("transcript.cancelled")) {
			t.Fatalf("%s: the mark outlived the next request", locale)
		}
	}
}
