package terminal

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestActionPaletteUsesListSelectionAndFilter(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = update(m, ControlIntent("palette"))
	if len(m.Palette.List.Items()) != 11 {
		t.Fatal("palette did not load its actions into the list component")
	}
	m = update(m, tea.KeyPressMsg{Code: '/'})
	if !m.Palette.List.SettingFilter() || m.View().Cursor == nil {
		t.Fatal("filter did not take focus")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.overlay != "palette" || m.Palette.List.SettingFilter() {
		t.Fatal("escape in filter closed the palette")
	}
	m.Palette.List.SetFilterText("help")
	selected, ok := m.Palette.List.SelectedItem().(actionItem)
	if len(m.Palette.List.VisibleItems()) >= len(actionItems) || !ok || selected.intent != "help" {
		t.Fatalf("list filter did not prioritize help: %v", m.Palette.List.VisibleItems())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.overlay != "help" {
		t.Fatalf("enter did not open selected action: %s", m.overlay)
	}
}

func TestActionPaletteKeepsKeyboardShortcutsAndTheme(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	m := New(Dependencies{UIPreferences: domain.UIPreferences{Theme: domain.ThemeInk, Icons: domain.IconsSafe}})
	defer m.zones.Close()
	m = update(m, tea.WindowSizeMsg{Width: 70, Height: 20})
	m = update(m, ControlIntent("palette"))
	view := m.View().Content
	if !strings.Contains(view, "Actions") || !strings.Contains(view, "48;2;") || !strings.Contains(view, "Sessions") {
		t.Fatalf("palette lacks styled list or visible actions: %q", view)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.Palette.List.Index() != 1 {
		t.Fatal("arrow did not move the list selection")
	}
	m = update(m, tea.KeyPressMsg{Code: 'h', Text: "h"})
	if m.overlay != "help" {
		t.Fatal("palette shortcut stopped working")
	}
	m = update(m, ControlIntent("palette"))
	m.Palette.List.Select(6)
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.overlay != "help" {
		t.Fatal("enter did not activate selected list row")
	}
	if m.Palette.List.FilterState() == list.Filtering {
		t.Fatal("palette reopened with stale filter state")
	}
}
