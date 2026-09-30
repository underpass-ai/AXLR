package terminal

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// PluginPanel composes a selector and a scrollable tool inventory for both views.
type PluginPanel struct {
	Items    []domain.PluginState
	Selected int
	Loading  bool
	Error    string
	Details  viewport.Model
}

func NewPluginPanel() PluginPanel { return PluginPanel{Details: viewport.New()} }
func (p *PluginPanel) SetItems(items []domain.PluginState) {
	p.Items = items
	p.Selected = min(p.Selected, max(0, len(items)-1))
	p.Loading = false
	p.Error = ""
	p.refresh()
}
func (p *PluginPanel) refresh() {
	if len(p.Items) == 0 {
		p.Details.SetContent("No plugins configured. Add an explicit manifest to mcp.json.")
		return
	}
	item := p.Items[p.Selected]
	var b strings.Builder
	fmt.Fprintf(&b, "%s · %s\n%s\n", item.Profile.Name, item.Profile.Purpose, item.Profile.Description)
	if item.Error != "" {
		fmt.Fprintf(&b, "Unavailable: %s\n", item.Error)
	} else {
		fmt.Fprintf(&b, "Connected · %d tools\n", len(item.Tools))
	}
	for _, tool := range item.Tools {
		fmt.Fprintf(&b, "  %s\n", tool)
	}
	p.Details.SetContent(Sanitize(b.String()))
	p.Details.GotoTop()
}
func (p *PluginPanel) Resize(w, h int) {
	p.Details.SetWidth(max(1, w))
	p.Details.SetHeight(max(1, h-min(len(p.Items), max(1, h/3))-4))
}
func (p *PluginPanel) Update(msg tea.Msg, mode ControlIntent) ControlIntent {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "up":
			p.Selected = max(0, p.Selected-1)
			p.refresh()
			return ""
		case "down":
			p.Selected = min(max(0, len(p.Items)-1), p.Selected+1)
			p.refresh()
			return ""
		case "r":
			return "plugins-refresh"
		case "a":
			if mode == "plugins" {
				return "plugins-toggle"
			}
			return ""
		}
	}
	p.Details, _ = p.Details.Update(msg)
	return ""
}
func (p PluginPanel) View(mode ControlIntent, w, h int) string {
	title := "MCP servers"
	if mode == "plugins" {
		title = "Plugins · explicit approval policies"
	}
	lines := []string{title}
	if p.Loading {
		lines = append(lines, "Loading…")
	}
	if p.Error != "" {
		lines = append(lines, "Error: "+singleLine(p.Error))
	}
	page := min(len(p.Items), max(1, h/3))
	start := min(max(0, p.Selected-page+1), max(0, len(p.Items)-page))
	for i := start; i < start+page; i++ {
		item := p.Items[i]
		selected := "  "
		if i == p.Selected {
			selected = "> "
		}
		state := "connected"
		if item.Error != "" {
			state = "unavailable"
		}
		lines = append(lines, fmt.Sprintf("%s%s · %s · %d tools · approval: %s", selected, item.Profile.ID, state, len(item.Tools), item.Profile.Approval))
	}
	lines = append(lines, p.Details.View())
	footer := "↑↓ Plugin · PgUp/PgDn Details · R Refresh · Esc Close"
	if mode == "plugins" {
		footer = "↑↓ Plugin · A Autoapprove on/off · R Refresh · Esc Close"
	}
	lines = append(lines, footer)
	rendered := strings.Split(Sanitize(strings.Join(lines, "\n")), "\n")
	if len(rendered) > h {
		rendered = rendered[:h-1]
		rendered = append(rendered, footer)
	}
	for i, line := range rendered {
		rendered[i] = ansi.Truncate(line, w, "…")
	}
	return strings.Join(rendered, "\n")
}
