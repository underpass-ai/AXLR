package terminal

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// /clean-screen empties the screen and keeps the session: the messages, the
// last reply /copy takes, and what is said afterwards, drawn alone.
func TestCleanScreenClearsTheScreenAndKeepsTheSession(t *testing.T) {
	for _, command := range []string{"/clean-screen", "/Clean-Screen", "/clean_screen", "/cls", "/limpiar"} {
		m := conversation(t)
		m.Header.State.ID = "s1"
		m.Header.State.ConsoleLines = append(m.Header.State.ConsoleLines, domain.ConsoleLine{AfterMessage: 2, Failure: &domain.TurnFailure{Message: "console note"}})
		m.refreshTranscript()
		if view := ansi.Strip(m.View().Content); !strings.Contains(view, "question") || !strings.Contains(view, "console note") {
			t.Fatalf("before %s:\n%s", command, view)
		}
		m.Composer.Input.SetValue(command)
		next, cmd := m.Update(ControlIntent("send"))
		m = next.(AppModel)
		view := ansi.Strip(m.View().Content)
		if cmd != nil || m.Busy || len(m.Header.State.Messages) != 2 || m.Composer.Input.Value() != "" {
			t.Fatalf("%s: cmd=%v busy=%v messages=%d draft=%q", command, cmd != nil, m.Busy, len(m.Header.State.Messages), m.Composer.Input.Value())
		}
		for _, gone := range []string{"question", "alpha beta", "console note"} {
			if strings.Contains(view, gone) {
				t.Fatalf("%s left %q on the screen:\n%s", command, gone, view)
			}
		}
		if !strings.Contains(view, "Screen cleared") {
			t.Fatalf("%s gave no notice:\n%s", command, view)
		}
		// The session still holds the last reply.
		m.Composer.Input.SetValue("/copy")
		next, cmd = m.Update(ControlIntent("send"))
		m = next.(AppModel)
		copied(t, cmd, "alpha beta\n\ngamma delta")
		// What follows is drawn.
		m.Header.State.Messages = append(m.Header.State.Messages, root.Message{Role: root.RoleUser, Content: "second question"}, root.Message{Role: root.RoleAssistant, Content: "second reply"})
		m.Header.State.ConsoleLines = append(m.Header.State.ConsoleLines, domain.ConsoleLine{AfterMessage: 2, Failure: &domain.TurnFailure{Message: "later note"}})
		m.refreshTranscript()
		view = ansi.Strip(m.View().Content)
		for _, shown := range []string{"later note", "second question", "second reply"} {
			if !strings.Contains(view, shown) {
				t.Fatalf("after %s, %q is not drawn:\n%s", command, shown, view)
			}
		}
		if strings.Contains(view, "alpha beta") || strings.Contains(view, "console note") {
			t.Fatalf("after %s the cleared rows came back:\n%s", command, view)
		}
		// Another session is drawn whole.
		m.Header.State.ID = "s2"
		m.refreshTranscript()
		if view := ansi.Strip(m.View().Content); !strings.Contains(view, "alpha beta") {
			t.Fatalf("another session after %s is cut:\n%s", command, view)
		}
	}
}

// A mistyped /clean_screen alias is refused, never a paid prompt.
func TestCleanScreenTypoIsRefused(t *testing.T) {
	if word, unknown := unknownSlashWord("/clean_scren", ""); !unknown || word != "/clean_scren" {
		t.Fatalf("/clean_scren read as unknown=%v %q", unknown, word)
	}
}
