package terminal

import "github.com/underpass-ai/AXLR/tui/domain"

func (m *AppModel) applyUIPreferences(p domain.UIPreferences) {
	m.Theme.ID = p.Theme
	m.Theme.Icons = p.Icons
	m.Composer.Theme = m.Theme
	m.Composer.Input.Placeholder = m.Theme.T("composer.placeholder")
	m.Models.Theme = m.Theme
	m.Changes.Theme = m.Theme
	m.Changes.Resize(m.Layout.Width, m.Layout.Height-1)
	m.Models.Input.Prompt = m.Theme.T("common.searchPrompt")
	m.Plugins.Theme = m.Theme
	m.InstalledPlugins.Theme = m.Theme
	m.Plugins.Search.Placeholder = m.Theme.T("plugins.searchPlaceholder")
	m.Transcript.ApplyTheme(m.Theme)
}
