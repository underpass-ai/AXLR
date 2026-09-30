package terminal

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/application"
)

// InstalledPlugins is the AXLR package catalog, separate from MCP connections.
type InstalledPlugins struct {
	Theme                 Theme
	Items                 []application.InstalledPlugin
	Selected              int
	Search                textinput.Model
	MarketplaceInput      textinput.Model
	Details               viewport.Model
	Loading               bool
	Error                 string
	Available             bool
	loadedAvailable       bool
	addingMarketplace     bool
	searching, confirming bool
	w, h                  int
}

func NewInstalledPlugins() InstalledPlugins {
	search := textinput.New()
	search.Placeholder = Translate(English, "catalog.search")
	marketplace := textinput.New()
	marketplace.Placeholder = Translate(English, "catalog.marketplacePlaceholder")
	return InstalledPlugins{Search: search, MarketplaceInput: marketplace, Details: viewport.New()}
}

func (p *InstalledPlugins) Resize(w, h int) {
	p.w, p.h = max(1, w), max(1, h)
	p.Search.SetWidth(max(1, w-12))
	p.MarketplaceInput.SetWidth(max(1, w-12))
	dw := w
	if w >= 80 {
		dw = w - max(32, w*42/100) - 2
	}
	p.Details.SetWidth(max(1, dw))
	p.Details.SetHeight(max(1, h-6))
}

func (p *InstalledPlugins) SetItems(items []application.InstalledPlugin) {
	id := ""
	if p.Selected >= 0 && p.Selected < len(p.Items) {
		id = p.Items[p.Selected].ID
	}
	p.Items = items
	if p.Available {
		p.loadedAvailable = true
	}
	p.Selected = 0
	for i, item := range items {
		if item.ID == id {
			p.Selected = i
			break
		}
	}
	p.Loading, p.Error = false, ""
	p.ensureVisible()
	p.refresh()
}

func (p *InstalledPlugins) visible() []int {
	q := strings.ToLower(strings.TrimSpace(p.Search.Value()))
	var result []int
	for i, item := range p.Items {
		if !p.Available && !item.Installed {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(item.ID+" "+item.Name+" "+item.Source), q) {
			continue
		}
		result = append(result, i)
	}
	return result
}

func (p *InstalledPlugins) ensureVisible() {
	visible := p.visible()
	for _, i := range visible {
		if i == p.Selected {
			return
		}
	}
	if len(visible) > 0 {
		p.Selected = visible[0]
	}
}

func (p *InstalledPlugins) refresh() {
	visible := p.visible()
	if len(visible) == 0 {
		p.Details.SetContent(p.Theme.T("catalog.noSelection"))
		return
	}
	item := p.Items[p.Selected]
	status := p.Theme.T("catalog.available")
	if item.Installed {
		status = p.Theme.T("catalog.installed")
	}
	content := fmt.Sprintf("%s\n\n%s\n%s\n%s", item.Name, p.Theme.Tf("catalog.id", item.ID), p.Theme.Tf("catalog.version", item.Version), p.Theme.Tf("catalog.status", status))
	if item.Description != "" {
		content += "\n\n" + item.Description
	}
	if len(item.Components) > 0 {
		labels := make([]string, 0, len(item.Components))
		for _, component := range item.Components {
			labels = append(labels, catalogEnum(p.Theme, "catalog.component.", component))
		}
		content += "\n\n" + p.Theme.Tf("catalog.components", strings.Join(labels, " · "))
	}
	if item.Source != "" {
		content += "\n" + p.Theme.Tf("catalog.source", item.Source)
	}
	p.Details.SetContent(Sanitize(content))
	p.Details.GotoTop()
}

func catalogEnum(theme Theme, prefix, value string) string {
	key := prefix + value
	if label := theme.T(key); label != key {
		return label
	}
	return value
}

func (p *InstalledPlugins) Update(msg tea.Msg) ControlIntent {
	if p.Loading {
		return ""
	}
	if p.addingMarketplace {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				p.addingMarketplace = false
				p.MarketplaceInput.Blur()
				return ""
			case "enter":
				if strings.TrimSpace(p.MarketplaceInput.Value()) == "" {
					return ""
				}
				p.addingMarketplace = false
				p.MarketplaceInput.Blur()
				return "catalog-marketplace"
			}
		}
		p.MarketplaceInput, _ = p.MarketplaceInput.Update(msg)
		return ""
	}
	if p.confirming {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "enter":
				p.confirming = false
				return "catalog-change"
			case "esc":
				p.confirming = false
			}
		}
		return ""
	}
	if p.searching {
		if key, ok := msg.(tea.KeyPressMsg); ok && (key.String() == "esc" || key.String() == "enter") {
			p.searching = false
			p.Search.Blur()
			return ""
		}
		p.Search, _ = p.Search.Update(msg)
		p.ensureVisible()
		p.refresh()
		return ""
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "/":
			p.searching = true
			p.Search.Focus()
			return ""
		case "tab":
			p.Available = !p.Available
			p.ensureVisible()
			p.refresh()
			if p.Available && !p.loadedAvailable {
				return "catalog-refresh"
			}
			return ""
		case "up", "down":
			visible := p.visible()
			if len(visible) == 0 {
				return ""
			}
			at := 0
			for i, index := range visible {
				if index == p.Selected {
					at = i
					break
				}
			}
			if key.String() == "up" {
				at = max(0, at-1)
			} else {
				at = min(len(visible)-1, at+1)
			}
			p.Selected = visible[at]
			p.refresh()
			return ""
		case "r":
			return "catalog-refresh"
		case "m":
			p.addingMarketplace = true
			p.MarketplaceInput.Reset()
			p.MarketplaceInput.Focus()
			return ""
		case "enter", "i":
			if len(p.visible()) > 0 && p.Selected < len(p.Items) && !p.Items[p.Selected].Installed {
				p.confirming = true
			}
			return ""
		}
	}
	p.Details, _ = p.Details.Update(msg)
	return ""
}

func (p InstalledPlugins) View(w, h int) string {
	w, h = max(1, w), max(1, h)
	p.Search.Placeholder = p.Theme.T("catalog.search")
	mode := p.Theme.T("catalog.installedTab")
	if p.Available {
		mode = p.Theme.T("catalog.allTab")
	}
	lines := []string{p.Theme.Heading(p.Theme.T("catalog.title")), p.Search.View(), p.Theme.Muted(p.Theme.Tf("catalog.filter", mode))}
	if p.addingMarketplace {
		lines = append(lines, p.Theme.T("catalog.marketplacePrompt"), p.MarketplaceInput.View())
	}
	if p.Loading {
		lines = append(lines, p.Theme.Muted(p.Theme.T("catalog.loading")))
	}
	if p.Error != "" {
		lines = append(lines, p.Theme.T("status.error")+singleLine(p.Error))
	}
	visible := p.visible()
	wide := w >= 80
	listWidth := w
	if wide {
		listWidth = max(32, w*42/100)
	}
	page := max(1, (h-5)/2)
	if !wide {
		page = max(1, (h-8)/3)
	}
	var left []string
	if len(visible) == 0 {
		left = append(left, p.Theme.T("catalog.empty"))
	} else {
		at := 0
		for i, index := range visible {
			if index == p.Selected {
				at = i
				break
			}
		}
		start := max(0, at-page+1)
		for _, index := range visible[start:min(len(visible), start+page)] {
			item := p.Items[index]
			status := p.Theme.T("catalog.available")
			if item.Installed {
				status = p.Theme.T("catalog.installed")
			}
			row := fmt.Sprintf("%s · %s", singleLine(item.Name), status)
			if index == p.Selected {
				row = p.Theme.Selected("› " + row)
			} else {
				row = "  " + row
			}
			left = append(left, ansi.Truncate(row, listWidth, "…"))
			left = append(left, p.Theme.Muted(ansi.Truncate("  "+singleLine(item.ID)+" · "+singleLine(item.Version), listWidth, "…")))
		}
	}
	detail := p.Details.View()
	if p.confirming && len(visible) > 0 {
		item := p.Items[p.Selected]
		detail = p.Theme.Tf("catalog.confirmInstall", item.ID)
		detail += "\n\n" + p.Theme.T("catalog.confirmHint")
	}
	if wide {
		leftBody := lipgloss.NewStyle().Width(listWidth).Height(max(1, h-5)).Render(strings.Join(left, "\n"))
		rightBody := p.Theme.Panel(lipgloss.NewStyle().Height(max(1, h-5)).Render(detail), max(1, w-listWidth-2))
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, leftBody, "  ", rightBody))
	} else {
		lines = append(lines, left...)
		if len(visible) > 0 {
			lines = append(lines, detail)
		}
	}
	lines = append(lines, p.Theme.Muted(p.Theme.T("catalog.footer")))
	rendered := strings.Split(strings.Join(lines, "\n"), "\n")
	if len(rendered) > h {
		rendered = append(rendered[:h-1], rendered[len(rendered)-1])
	}
	for i, line := range rendered {
		rendered[i] = ansi.Truncate(line, w, "…")
	}
	return strings.Join(rendered, "\n")
}
