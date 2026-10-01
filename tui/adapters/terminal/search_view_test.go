package terminal

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
