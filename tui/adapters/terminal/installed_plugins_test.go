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
func (s *pluginCatalogStub) AddMarketplace(_ context.Context, source string) error {
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

func TestUnavailablePluginCannotBeInstalled(t *testing.T) {
	p := NewInstalledPlugins()
	p.Theme = Theme{Locale: English, Monochrome: true}
	p.Available = true
	p.SetItems([]application.InstalledPlugin{{ID: "blocked@market", Name: "Blocked", InstallPolicy: "NOT_AVAILABLE"}})
	if got := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); got != "" || p.confirming {
		t.Fatal("unavailable plugin was offered for installation")
	}
	if !strings.Contains(p.View(80, 20), "Unavailable") {
		t.Fatal("availability not shown")
	}
}
