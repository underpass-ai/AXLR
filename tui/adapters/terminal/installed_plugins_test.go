package terminal

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
)

type pluginCatalogStub struct {
	installed, all           []application.InstalledPlugin
	installedID, marketplace string
	updatedID, removedID     string
}

func (s *pluginCatalogStub) List(_ context.Context, available bool) ([]application.InstalledPlugin, error) {
	if available {
		return s.all, nil
	}
	return s.installed, nil
}
func (s *pluginCatalogStub) Install(_ context.Context, id string) error {
	s.installedID = id
	for i := range s.all {
		if s.all[i].ID == id {
			s.all[i].Installed = true
		}
	}
	return nil
}
func (s *pluginCatalogStub) AddSource(_ context.Context, source string) error {
	s.marketplace = source
	return nil
}

func TestPluginCommandUsesPackageCatalogAndInstalls(t *testing.T) {
	stub := &pluginCatalogStub{installed: []application.InstalledPlugin{{ID: "kmp@underpass", Name: "KMP", Installed: true}}, all: []application.InstalledPlugin{{ID: "kmp@underpass", Name: "KMP", Installed: true}, {ID: "sample@third-party", Name: "Sample"}}}
	m := sized()
	defer m.zones.Close()
	m.deps.InstalledPlugins = stub
	m.Composer.Input.SetValue("/plugin")
	next, cmd := m.Update(ControlIntent("send"))
	m = runUIOperation(next.(AppModel), cmd)
	if m.overlay != "plugins" || !strings.Contains(m.View().Content, "kmp@underpass") || strings.Contains(m.View().Content, "kmp_wake") {
		t.Fatal("/plugin did not open the package catalog")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = runUIOperation(next.(AppModel), cmd)
	if !strings.Contains(m.View().Content, "sample@third-party") {
		t.Fatal("available package not shown")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(AppModel)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(AppModel)
	if cmd != nil || stub.installedID != "" || !strings.Contains(m.View().Content, "Install sample@third-party") {
		t.Fatal("install did not require review")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = runUIOperation(next.(AppModel), cmd)
	if stub.installedID != "sample@third-party" || !m.InstalledPlugins.Items[1].Installed {
		t.Fatal("exact package was not installed")
	}
}

func TestPluginMarketplaceInput(t *testing.T) {
	stub := &pluginCatalogStub{}
	m := sized()
	defer m.zones.Close()
	m.deps.InstalledPlugins = stub
	next, cmd := m.Update(ControlIntent("plugins"))
	m = runUIOperation(next.(AppModel), cmd)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = next.(AppModel)
	m.InstalledPlugins.MarketplaceInput.SetValue("https://github.com/example/plugins")
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = runUIOperation(next.(AppModel), cmd)
	if stub.marketplace != "https://github.com/example/plugins" || !m.InstalledPlugins.Available {
		t.Fatal("marketplace was not added")
	}
}

func (s *pluginCatalogStub) Guidance(context.Context) (string, error) { return "", nil }
func (s *pluginCatalogStub) Update(_ context.Context, id string) error {
	s.updatedID = id
	return nil
}
func (s *pluginCatalogStub) Uninstall(_ context.Context, id string) error {
	s.removedID = id
	kept := s.installed[:0:0]
	for _, item := range s.installed {
		if item.ID != id {
			kept = append(kept, item)
		}
	}
	s.installed = kept
	return nil
}

// U and X act on the selected installed package after a review; built-in
// entries are offered neither.
func TestPluginPanelUpdatesAndRemovesAnInstalledPackage(t *testing.T) {
	builtin := application.InstalledPlugin{ID: "kmp", Name: "KMP", Installed: true, Builtin: true}
	sample := application.InstalledPlugin{ID: "sample", Name: "Sample", Version: "1.0.0", Source: "/src/sample", Installed: true}
	stub := &pluginCatalogStub{installed: []application.InstalledPlugin{builtin, sample}}
	m := sized()
	defer m.zones.Close()
	m.deps.InstalledPlugins = stub
	next, cmd := m.Update(ControlIntent("plugins"))
	m = runUIOperation(next.(AppModel), cmd)
	for _, key := range []rune{'u', 'x'} {
		next, cmd = m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		m = next.(AppModel)
		if cmd != nil || m.InstalledPlugins.confirming {
			t.Fatalf("%c offered for a built-in entry", key)
		}
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(AppModel)
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m = next.(AppModel)
	if cmd != nil || stub.updatedID != "" || !strings.Contains(m.View().Content, "Update sample from /src/sample?") {
		t.Fatalf("update did not require review:\n%s", m.View().Content)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = runUIOperation(next.(AppModel), cmd)
	if stub.updatedID != "sample" {
		t.Fatal("exact package was not updated")
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(AppModel)
	if cmd != nil || !strings.Contains(m.View().Content, "Remove sample from AXLR?") {
		t.Fatalf("removal did not require review:\n%s", m.View().Content)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runUIOperation(next.(AppModel), cmd)
	if stub.removedID != "" || m.InstalledPlugins.confirming {
		t.Fatal("Esc did not cancel the removal")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = next.(AppModel)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = runUIOperation(next.(AppModel), cmd)
	if stub.removedID != "sample" || len(m.InstalledPlugins.Items) != 1 {
		t.Fatalf("package was not removed: %#v", m.InstalledPlugins.Items)
	}
}

func TestBuiltinPluginCannotBeInstalled(t *testing.T) {
	p := NewInstalledPlugins()
	p.Theme = Theme{Locale: English, Monochrome: true}
	p.Available = true
	p.SetItems([]application.InstalledPlugin{{ID: "kmp", Name: "KMP", Installed: true, Builtin: true}})
	if got := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); got != "" || p.confirming {
		t.Fatal("unavailable plugin was offered for installation")
	}
	if !strings.Contains(p.View(80, 20), "Installed") {
		t.Fatal("availability not shown")
	}
}
