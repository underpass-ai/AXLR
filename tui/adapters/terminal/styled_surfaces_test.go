package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestStyledSurfacesStayWithinTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	m := New(Dependencies{Session: &s, UIPreferences: domain.UIPreferences{Theme: domain.ThemeInk, Icons: domain.IconsSafe}})
	defer m.zones.Close()
	for _, size := range [][2]int{{50, 15}, {100, 32}} {
		m = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, overlay := range []ControlIntent{"palette", "help", "sessions", "info"} {
			if overlay == "sessions" {
				m.overlay = "sessions"
				m.Picker = SessionPicker{Items: []domain.SessionSummary{{ID: s.Export().ID, Model: s.Export().Model, Workspace: s.Export().Workspace}}}
			} else {
				m = update(m, overlay)
			}
			view := m.View().Content
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("%s overflow at %v: %dx%d: %q", overlay, size, lipgloss.Width(view), lipgloss.Height(view), view)
			}
			if !strings.Contains(view, "48;2;") {
				t.Fatalf("%s has no surface color", overlay)
			}
			m = update(m, ControlIntent("close"))
		}
		view := m.View().Content
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("main overflow at %v", size)
		}
	}
}

func TestTranscriptAddsAirWithoutChangingSavedMessages(t *testing.T) {
	state := domain.SessionState{Messages: []root.Message{{Role: root.RoleUser, Content: "question"}, {Role: root.RoleAssistant, Content: "answer"}}}
	tr := NewTranscript()
	tr.Gutter = 2
	tr.Viewport.SetWidth(46)
	tr.Viewport.SetHeight(6)
	tr.SetSession(state, "", Theme{ID: domain.ThemeInk})
	if !strings.Contains(tr.Viewport.GetContent(), "question\n\nassistant: answer") {
		t.Fatal("conversation groups lack spacing")
	}
	if !strings.Contains(tr.View(), "  user: question") || lipgloss.Width(tr.View()) != 50 {
		t.Fatalf("transcript gutter or width lost: %q (%d)", tr.View(), lipgloss.Width(tr.View()))
	}
	if state.Messages[0].Content != "question" || state.Messages[1].Content != "answer" {
		t.Fatal("presentation altered history")
	}
}

func TestInfoFormatsJSONWithoutDroppingToolResult(t *testing.T) {
	value := `{"status":"completed","result":{"count":2}}`
	formatted := prettyToolResult(value)
	if !strings.Contains(formatted, "\n  \"result\"") || !strings.Contains(formatted, "\"count\": 2") {
		t.Fatalf("unreadable JSON: %s", formatted)
	}
	preview := toolResultSummary(value)
	if strings.Contains(preview, "count") || !strings.Contains(preview, "completed") || !strings.Contains(preview, "full result saved") {
		t.Fatalf("tool result preview is too dense or lacks a full-result path: %s", preview)
	}
}
