package terminal

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
)

// The wheel belongs to what is on screen: an overlay's own content, never
// the conversation hidden behind it.
func TestMouseWheelScrollsTheOverlayNotTheHiddenConversation(t *testing.T) {
	long := strings.Repeat("row\n", 80)
	s := navSession(t)
	if err := s.BeginTurn("question", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: root.Text(long)}}); err != nil {
		t.Fatal(err)
	}
	wheel := tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 10, Y: 8}
	for _, overlay := range []ControlIntent{"info", "approvals", "updates", "made-setup", "plans", "repairs", "incident"} {
		m := navModel(t, &s)
		m.Transcript.Viewport.GotoTop()
		m = update(m, ControlIntent("info"))
		m.Info.SetContent(long)
		m.Info.Viewport.GotoTop()
		m.overlay = overlay
		m = update(m, wheel)
		if m.Transcript.Viewport.YOffset() != 0 {
			t.Fatalf("%s: the wheel scrolled the hidden conversation to %d", overlay, m.Transcript.Viewport.YOffset())
		}
		if m.Info.Viewport.YOffset() == 0 {
			t.Fatalf("%s: the wheel did not scroll the overlay", overlay)
		}
		m.zones.Close()
	}
	for _, overlay := range []ControlIntent{"help", "sessions"} {
		m := navModel(t, &s)
		m.Transcript.Viewport.GotoTop()
		m.overlay = overlay
		m = update(m, wheel)
		if m.Transcript.Viewport.YOffset() != 0 {
			t.Fatalf("%s: the wheel scrolled the hidden conversation to %d", overlay, m.Transcript.Viewport.YOffset())
		}
		m.zones.Close()
	}
	// Search shows the conversation itself, so the wheel still moves it.
	m := navModel(t, &s)
	defer m.zones.Close()
	m.Transcript.Viewport.GotoTop()
	m = update(m, ControlIntent("search"))
	if m = update(m, wheel); m.Transcript.Viewport.YOffset() == 0 {
		t.Fatal("the wheel no longer scrolls the conversation during search")
	}
}

// On 14 October 2026 the info panel (Ctrl+P → I) scrolled with PgUp and
// PgDn but ignored Home and End; /usage, /session and the /mcp detail did
// too.
func TestHomeAndEndJumpInScrollingPanels(t *testing.T) {
	long := strings.Repeat("row\n", 80)
	for _, overlay := range []ControlIntent{"info", "session", "usage", "memory"} {
		for _, keys := range [][2]tea.KeyPressMsg{{{Code: tea.KeyEnd}, {Code: tea.KeyHome}}, {{Code: tea.KeyEnd, Mod: tea.ModCtrl}, {Code: tea.KeyHome, Mod: tea.ModCtrl}}} {
			m := sized()
			m.overlay = overlay
			m.Info.SetContent(long)
			m.Info.Viewport.GotoTop()
			if m = update(m, keys[0]); !m.Info.Viewport.AtBottom() {
				t.Fatalf("%s: %s did not reach the end", overlay, keys[0].String())
			}
			if m = update(m, keys[1]); m.Info.Viewport.YOffset() != 0 {
				t.Fatalf("%s: %s did not reach the top", overlay, keys[1].String())
			}
		}
	}
	p := PluginPanel{Details: viewport.New(viewport.WithWidth(40), viewport.WithHeight(5))}
	p.Details.SetContent(long)
	if p.Update(tea.KeyPressMsg{Code: tea.KeyEnd}, "mcp"); !p.Details.AtBottom() {
		t.Fatal("/mcp: End did not reach the end of the detail")
	}
	if p.Update(tea.KeyPressMsg{Code: tea.KeyHome}, "mcp"); p.Details.YOffset() != 0 {
		t.Fatal("/mcp: Home did not reach the top of the detail")
	}
}
