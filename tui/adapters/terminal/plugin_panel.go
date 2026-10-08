package terminal

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// PluginPanel keeps the selected manifest identity separate from list filters.
type PluginPanel struct {
	Theme        Theme
	Items        []domain.PluginState
	Selected     int
	Loading      bool
	Error        string
	Details      viewport.Model
	Search       textinput.Model
	InstallInput textinput.Model
	searching    bool
	installing   bool
	confirming   bool
	filter       int // all, connected, unavailable, manual, automatic
	w, h         int
}

func NewPluginPanel() PluginPanel {
	search := textinput.New()
	search.Placeholder = Translate(English, "plugins.searchPlaceholder")
	install := textinput.New()
	install.Placeholder = Translate(English, "mcp.manifestPlaceholder")
	return PluginPanel{Details: viewport.New(), Search: search, InstallInput: install}
}

func (p *PluginPanel) SetItems(items []domain.PluginState) {
	selectedID := ""
	if p.Selected >= 0 && p.Selected < len(p.Items) {
		selectedID = string(p.Items[p.Selected].Profile.ID)
	}
	p.Items = items
	p.Selected = 0
	for i, item := range items {
		if string(item.Profile.ID) == selectedID {
			p.Selected = i
			break
		}
	}
	p.Loading, p.Error = false, ""
	p.ensureVisible()
	p.refresh()
}

func (p *PluginPanel) visible(mode ControlIntent) []int {
	q := strings.ToLower(strings.TrimSpace(p.Search.Value()))
	var indices []int
	for i, item := range p.Items {
		switch p.filter {
		case 1:
			if item.Error != "" {
				continue
			}
		case 2:
			if item.Error == "" {
				continue
			}
		case 3:
			if mode == "mcp" && item.Profile.Approval != domain.ApprovalManual {
				continue
			}
		case 4:
			if mode == "mcp" && item.Profile.Approval != domain.ApprovalAuto {
				continue
			}
		}
		haystack := string(item.Profile.ID) + " " + string(item.Profile.Name) + " " + string(item.Profile.Purpose) + " " + string(item.Profile.Description)
		for _, tool := range item.Tools {
			haystack += " " + string(tool)
		}
		if q == "" || strings.Contains(strings.ToLower(haystack), q) {
			indices = append(indices, i)
		}
	}
	return indices
}

func (p *PluginPanel) ensureVisible() {
	visible := p.visible("mcp")
	if len(visible) == 0 {
		return
	}
	for _, i := range visible {
		if i == p.Selected {
			return
		}
	}
	p.Selected = visible[0]
}

func (p *PluginPanel) refresh() {
	if len(p.Items) == 0 {
		p.Details.SetContent(p.Theme.T("plugins.noServersDetail"))
		return
	}
	item := p.Items[p.Selected]
	var b strings.Builder
	fmt.Fprintf(&b, "%s · %s\n%s\n", item.Profile.Name, item.Profile.Purpose, item.Profile.Description)
	fmt.Fprintln(&b, p.Theme.Tf("plugins.exactID", item.Profile.ID))
	if item.Error != "" {
		fmt.Fprintln(&b, p.Theme.Tf("plugins.unavailable", item.Error))
	} else {
		fmt.Fprintln(&b, p.Theme.Tf("plugins.connectedCount", len(item.Tools)))
	}
	fmt.Fprint(&b, p.Theme.Tf("plugins.approvalDetails", p.Theme.T("approval."+string(item.Profile.Approval))))
	for _, tool := range item.Tools {
		fmt.Fprintf(&b, "  %s\n", tool)
	}
	if len(item.Tools) == 0 {
		b.WriteString(p.Theme.T("plugins.none"))
	}
	p.Details.SetContent(Sanitize(b.String()))
	p.Details.GotoTop()
}

func (p *PluginPanel) Resize(w, h int) {
	p.w, p.h = max(1, w), max(1, h)
	p.Search.SetWidth(max(1, w-12))
	p.InstallInput.SetWidth(max(1, w-12))
	detailWidth := w
	if w >= 80 {
		detailWidth = w - max(32, w*42/100) - 2
	}
	p.Details.SetWidth(max(1, detailWidth))
	detailHeight := h - 6
	if w < 80 {
		detailHeight = (h - 7) / 2
	}
	p.Details.SetHeight(max(1, detailHeight))
}

func (p *PluginPanel) Update(msg tea.Msg, mode ControlIntent) ControlIntent {
	if p.installing {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				p.installing = false
				p.InstallInput.Blur()
				return ""
			case "enter":
				if strings.TrimSpace(p.InstallInput.Value()) == "" {
					return ""
				}
				p.installing = false
				p.InstallInput.Blur()
				return "mcp-install"
			}
		}
		p.InstallInput, _ = p.InstallInput.Update(msg)
		return ""
	}
	if p.confirming {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "enter":
				p.confirming = false
				return "plugins-toggle"
			case "esc":
				p.confirming = false
				return ""
			}
		}
		return ""
	}
	if p.searching {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc", "enter":
				p.searching = false
				p.Search.Blur()
				return ""
			}
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
			limit := 2
			if mode == "mcp" {
				limit = 4
			}
			p.filter = (p.filter + 1) % (limit + 1)
			p.ensureVisible()
			p.refresh()
			return ""
		case "up", "down":
			visible := p.visible(mode)
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
		case "r", "R":
			return "plugins-refresh"
		case "p", "P":
			if mode == "mcp" && p.Selected < len(p.Items) && p.Items[p.Selected].Profile.ID == "made" {
				return "made-prepare"
			}
			return ""
		case "i", "I":
			p.installing = true
			p.InstallInput.Reset()
			p.InstallInput.Focus()
			return ""
		case "a", "A", "enter":
			if mode == "mcp" && len(p.Items) > 0 && len(p.visible(mode)) > 0 {
				p.confirming = true
			}
			return ""
		}
	}
	p.Details, _ = p.Details.Update(msg)
	return ""
}

func (p PluginPanel) View(mode ControlIntent, w, h int) string {
	w, h = max(1, w), max(1, h)
	p.Search.Placeholder = p.Theme.T("plugins.searchPlaceholder")
	title := p.Theme.T("plugins.mcpTitle")
	filters := []string{p.Theme.T("filters.all"), p.Theme.T("filters.connected"), p.Theme.T("filters.errors"), p.Theme.T("filters.manualTitle"), p.Theme.T("filters.autoTitle")}
	filter := filters[min(p.filter, len(filters)-1)]
	lines := []string{p.Theme.Heading(title), p.Search.View(), p.Theme.Muted(p.Theme.Tf("plugins.filterHint", filter))}
	if p.installing {
		lines = append(lines, p.Theme.T("mcp.manifestPrompt"), p.InstallInput.View())
	}
	if p.Loading {
		lines = append(lines, p.Theme.Muted(p.Theme.T("plugins.loading")))
	}
	if p.Error != "" {
		lines = append(lines, p.Theme.T("status.error")+singleLine(p.Error))
	}
	visible := p.visible(mode)
	listWidth := w
	wide := w >= 80
	if wide {
		listWidth = max(32, w*42/100)
	}
	page := max(1, (h-4)/2)
	if !wide {
		page = max(1, (h-7)/4)
	}
	var left []string
	if len(visible) == 0 {
		if len(p.Items) == 0 {
			left = append(left, p.Theme.T("plugins.empty"))
		} else {
			left = append(left, p.Theme.T("plugins.noResults"))
		}
	} else {
		selected := 0
		for i, index := range visible {
			if index == p.Selected {
				selected = i
				break
			}
		}
		start := max(0, selected-page+1)
		for _, index := range visible[start:min(len(visible), start+page)] {
			item := p.Items[index]
			state := p.Theme.Icon("connected")
			if item.Error != "" {
				state = p.Theme.Icon("error")
			}
			row := p.Theme.Tf("plugins.rowCount", state, singleLine(string(item.Profile.Name)), len(item.Tools))
			if index == p.Selected {
				row = p.Theme.Selected("› " + row)
			} else {
				row = "  " + row
			}
			left = append(left, ansi.Truncate(row, listWidth, "…"))
			left = append(left, p.Theme.Muted(ansi.Truncate("  "+singleLine(string(item.Profile.ID))+" · "+p.Theme.T("approval."+string(item.Profile.Approval)), listWidth, "…")))
		}
	}
	if wide {
		detailWidth := max(1, w-listWidth-2)
		leftBody := lipgloss.NewStyle().Width(listWidth).Height(max(1, h-4)).Render(strings.Join(left, "\n"))
		detail := p.Details.View()
		if len(visible) == 0 {
			detail = p.Theme.T("plugins.selectServer")
		}
		if p.confirming {
			item := p.Items[p.Selected]
			scope := "\n\n" + p.Theme.Tf("plugins.allowedCount", len(item.Tools))
			for _, name := range item.Tools[:min(3, len(item.Tools))] {
				scope += "\n  " + singleLine(string(name))
			}
			if item.Profile.Approval == domain.ApprovalManual {
				detail = p.Theme.T("plugins.autoApproval") + "\n\n" + p.Theme.Tf("plugins.autoExplanation", singleLine(string(item.Profile.ID))) + scope + "\n\n" + p.Theme.T("plugins.confirmHint")
			} else {
				detail = p.Theme.T("plugins.manualApproval") + "\n\n" + p.Theme.Tf("plugins.manualExplanation", singleLine(string(item.Profile.ID))) + scope + "\n\n" + p.Theme.T("plugins.confirmHint")
			}
		}
		rightBody := p.Theme.Panel(lipgloss.NewStyle().Height(max(1, h-4)).Render(detail), detailWidth)
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, leftBody, "  ", rightBody))
	} else {
		lines = append(lines, left...)
		if len(visible) > 0 {
			lines = append(lines, p.Details.View())
		}
	}
	footer := p.Theme.T("plugins.mcpFooter")
	if mode == "mcp" {
		footer = p.Theme.T("plugins.footer")
		if p.Selected < len(p.Items) && p.Items[p.Selected].Profile.ID == "made" {
			footer = p.Theme.T("plugins.madeFooter")
		}
	}
	if p.confirming && p.Selected < len(p.Items) {
		item := p.Items[p.Selected]
		next := p.Theme.T("approval.auto")
		if item.Profile.Approval == domain.ApprovalAuto {
			next = p.Theme.T("approval.manual")
		}
		footer = p.Theme.Tf("plugins.confirmFooter", p.Theme.T("approval."+string(item.Profile.Approval)), next, singleLine(string(item.Profile.ID)))
	}
	lines = append(lines, p.Theme.Muted(footer))
	rendered := strings.Split(strings.Join(lines, "\n"), "\n")
	if len(rendered) > h {
		rendered = append(rendered[:h-1], rendered[len(rendered)-1])
	}
	for i, line := range rendered {
		rendered[i] = ansi.Truncate(line, w, "…")
	}
	return strings.Join(rendered, "\n")
}
