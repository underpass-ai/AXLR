package terminal

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestMCPPanelPrepareAcceptsUppercaseP(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: 'p', Text: "p"}, {Code: 'P', Text: "P"}} {
		panel := NewPluginPanel()
		panel.SetItems([]domain.PluginState{pluginItem(root.PluginID("made"))})
		if got := panel.Update(key, "mcp"); got != "made-prepare" {
			t.Fatalf("key %q: got intent %q, want made-prepare", key.Text, got)
		}
	}
}

func TestMCPPanelUppercaseShortcutsMatchLowercase(t *testing.T) {
	panel := NewPluginPanel()
	panel.SetItems([]domain.PluginState{pluginItem(root.PluginID("made"))})
	if got := panel.Update(tea.KeyPressMsg{Code: 'R', Text: "R"}, "mcp"); got != "plugins-refresh" {
		t.Fatalf("R: got intent %q, want plugins-refresh", got)
	}

	panel = NewPluginPanel()
	panel.SetItems([]domain.PluginState{pluginItem(root.PluginID("made"))})
	panel.Update(tea.KeyPressMsg{Code: 'I', Text: "I"}, "mcp")
	if !panel.installing {
		t.Fatalf("I: install prompt did not open")
	}

	panel = NewPluginPanel()
	panel.SetItems([]domain.PluginState{pluginItem(root.PluginID("made"))})
	panel.Update(tea.KeyPressMsg{Code: 'A', Text: "A"}, "mcp")
	if !panel.confirming {
		t.Fatalf("A: approval confirmation did not open")
	}
}
