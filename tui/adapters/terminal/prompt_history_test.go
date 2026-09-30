package terminal

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestPromptHistoryUsesOnlyUserMessagesAndRestoresDraft(t *testing.T) {
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", "/tmp", "model")
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"first", "second\nline"} {
		if err := s.BeginTurn(root.Text(prompt), nil); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "assistant only"}}); err != nil {
			t.Fatal(err)
		}
	}
	m := update(New(Dependencies{Session: &s, Monochrome: true}), tea.WindowSizeMsg{Width: 80, Height: 25})
	m.Composer.Input.SetValue("current draft")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Composer.Input.Value() != "second\nline" {
		t.Fatalf("first history entry = %q", m.Composer.Input.Value())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Composer.Input.Value() != "first" {
		t.Fatalf("second history entry = %q", m.Composer.Input.Value())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.Composer.Input.Value() != "current draft" {
		t.Fatalf("draft was not restored: %q", m.Composer.Input.Value())
	}
}

func TestPromptHistoryLeavesMultilineCursorNavigation(t *testing.T) {
	m := sized()
	m.promptHistory = []string{"old"}
	m.historyIndex = 1
	m.Composer.Input.SetValue("top\nbottom")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Composer.Input.Value() != "top\nbottom" {
		t.Fatal("up replaced text while cursor was inside multiline draft")
	}
}
