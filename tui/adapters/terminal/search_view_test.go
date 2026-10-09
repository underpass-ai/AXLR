package terminal

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestSearchKeepsTheHeaderAndFooterAndReplacesTheComposer(t *testing.T) {
	m := update(sized(), ControlIntent("search"))
	defer m.zones.Close()
	m = update(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	v := m.View()
	plain := ansi.Strip(v.Content)
	lines := strings.Split(plain, "\n")
	if !strings.Contains(lines[0], "AXLR") {
		t.Fatalf("header lost while searching:\n%s", plain)
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, "enter next") || !strings.Contains(last, "esc close") || !strings.Contains(last, "0/0") {
		t.Fatalf("search footer missing: %q", last)
	}
	if strings.Contains(plain, "enter send") || lipgloss.Height(v.Content) != m.Layout.Height {
		t.Fatalf("composer still shown or height wrong:\n%s", plain)
	}
	if v.Cursor == nil || !strings.Contains(lines[v.Cursor.Y], "x") {
		t.Fatalf("cursor not on the search row")
	}
}

func TestSearchHighlightsEveryMatchInTheConversation(t *testing.T) {
	line := highlightMatches("  Hay 18 Ceremonias; ceremonias más", "ceremonias", Theme{Monochrome: true})
	if strings.Count(line, "\x1b[7m") != 2 || ansi.Strip(line) != "  Hay 18 Ceremonias; ceremonias más" {
		t.Fatalf("highlighted line = %q", line)
	}
	if highlightMatches("nada", "ceremonias", Theme{Monochrome: true}) != "nada" {
		t.Fatal("a line without matches changed")
	}
}

// A hit is measured with the transcript's own theme: Editorial adds speaker
// rows that a plain render does not have, so the jump fell short.
func TestSearchShowsTheHitInEveryTheme(t *testing.T) {
	for _, theme := range []domain.ThemeID{domain.ThemeInk, domain.ThemeEditorial} {
		s := navSession(t)
		for i := range 20 {
			prompt := root.Text(fmt.Sprintf("question %d", i))
			if i == 10 {
				prompt = "the needle question"
			}
			if err := s.BeginTurn(prompt, nil); err != nil {
				t.Fatal(err)
			}
			if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: root.Text(fmt.Sprintf("answer %d\nsecond line", i))}}); err != nil {
				t.Fatal(err)
			}
		}
		m := navModel(t, &s)
		m.applyUIPreferences(domain.UIPreferences{Theme: theme, Icons: domain.IconsSafe})
		m.refreshTranscript()
		m = update(m, ControlIntent("search"))
		m = typeDraft(m, "needle") // keys go to the search box
		if view := m.Transcript.View(); !strings.Contains(view, "needle") {
			t.Fatalf("%s: the hit is not on screen (offset %d of %d lines):\n%s", theme, m.Transcript.Viewport.YOffset(), m.Transcript.VisualLineCount(), view)
		}
		m.zones.Close()
	}
}
