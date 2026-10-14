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
	// Idle: continue and cancel do not apply.
	if len(m.Palette.List.Items()) != len(actionItems)-2 {
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
	m.Palette.List.Select(7)
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.overlay != "help" {
		t.Fatal("enter did not activate selected list row")
	}
	if m.Palette.List.FilterState() == list.Filtering {
		t.Fatal("palette reopened with stale filter state")
	}
}

// On 12 October 2026 clicking Jobs or Copy last reply in Ctrl+P did
// nothing, and Cancel while idle or Continue on a session that could not
// continue left the palette open with no message.
func TestEveryPaletteActionClosesItAndARefusedOneSaysWhy(t *testing.T) {
	open := func(t *testing.T) AppModel {
		s := navSession(t)
		m := navModel(t, &s)
		m = update(m, tea.WindowSizeMsg{Width: 100, Height: 45})
		return update(m, ControlIntent("palette"))
	}
	t.Run("click on Copy", func(t *testing.T) {
		m := open(t)
		n, _ := click(t, m, "copy")
		m = n.(AppModel)
		if m.overlay != "" || m.Status.Error != m.Theme.T("error.nothingToCopy") {
			t.Fatalf("copy click: overlay=%q error=%q", m.overlay, m.Status.Error)
		}
	})
	t.Run("click on Jobs", func(t *testing.T) {
		m := open(t)
		n, _ := click(t, m, "jobs")
		m = n.(AppModel)
		if m.overlay == "palette" || m.Status.Error != m.Theme.T("jobs.unavailable") {
			t.Fatalf("jobs click: overlay=%q error=%q", m.overlay, m.Status.Error)
		}
	})
	t.Run("click on the close button", func(t *testing.T) {
		m := open(t)
		n, _ := click(t, m, "close")
		if n.(AppModel).overlay != "" {
			t.Fatal("close did not close the palette")
		}
	})
	for _, refused := range []struct {
		key    rune
		busy   bool
		notice string
	}{
		{'c', false, "notice.nothingToCancel"},
		{'r', false, "notice.nothingToContinue"},
		{'o', true, "notice.sessionsBusy"},
	} {
		t.Run(string(refused.key), func(t *testing.T) {
			m := open(t)
			m.Busy = refused.busy
			m = update(m, tea.KeyPressMsg{Code: refused.key, Text: string(refused.key)})
			if m.overlay != "" || m.Status.Notice != m.Theme.T(refused.notice) {
				t.Fatalf("overlay=%q notice=%q", m.overlay, m.Status.Notice)
			}
			if !strings.Contains(m.View().Content, m.Theme.T(refused.notice)[:20]) {
				t.Fatalf("the reason is not on screen:\n%s", m.View().Content)
			}
		})
	}
	t.Run("models while busy", func(t *testing.T) {
		m := open(t)
		m.Busy = true
		m = update(m, tea.KeyPressMsg{Code: 'm', Text: "m"})
		if m.overlay != "" || m.Status.Error != m.Theme.T("error.modelBusy") {
			t.Fatalf("overlay=%q error=%q", m.overlay, m.Status.Error)
		}
	})
}

// On 14 October 2026 an idle console listed "Continue interrupted turn"
// and "Cancel turn" with nothing to continue or cancel.
func TestPaletteHidesActionsThatDoNotApply(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m = update(m, ControlIntent("palette"))
	listed := func(m AppModel) map[ControlIntent]bool {
		found := map[ControlIntent]bool{}
		for _, item := range m.Palette.List.Items() {
			found[item.(actionItem).intent] = true
		}
		return found
	}
	if got := listed(m); got["continue"] || got["cancel"] || !got["sessions"] {
		t.Fatalf("idle palette lists %v", got)
	}
	// Its shortcut still says why it does nothing.
	if n := update(m, tea.KeyPressMsg{Code: 'c', Text: "c"}); n.Status.Notice != n.Theme.T("notice.nothingToCancel") {
		t.Fatalf("a hidden action's shortcut said %q", n.Status.Notice)
	}
	if err := s.BeginTurn("go", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.InterruptDraft("partial"); err != nil {
		t.Fatal(err)
	}
	m.Header.State = s.Export()
	m = update(m, ControlIntent("palette"))
	if got := listed(m); !got["continue"] || !got["cancel"] {
		t.Fatalf("interrupted palette lists %v", got)
	}
}
