package terminal

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type changeRecord struct {
	ID     root.ToolCallID
	Change domain.FileChange
}

// ChangeViewer displays one recorded effect at a time, newest first. It owns
// review focus and scrolling; session snapshots remain its only data source.
type ChangeViewer struct {
	Theme         Theme
	Details       viewport.Model
	records       []changeRecord
	previews      []changePreview
	selected      int
	window        int
	detailFocus   bool
	width, height int
	sessionID     domain.SessionID
}

func NewChangeViewer() ChangeViewer {
	return ChangeViewer{Details: viewport.New()}
}

func (v *ChangeViewer) SetSession(state domain.SessionState) {
	var records []changeRecord
	for i := len(state.Activity) - 1; i >= 0; i-- {
		a := state.Activity[i]
		if a.Outcome != nil && !a.Outcome.IsError && !a.Outcome.Uncertain && a.Decision != domain.DecisionDeny && a.Outcome.Change != nil {
			records = append(records, changeRecord{ID: a.Call.ID, Change: *a.Outcome.Change})
		}
	}
	if state.ID == v.sessionID && slices.Equal(records, v.records) {
		return
	}
	selectedID := root.ToolCallID("")
	if state.ID == v.sessionID && len(v.records) > 0 {
		selectedID = v.records[v.selected].ID
	} else {
		v.detailFocus = false
		v.Details.GotoTop()
	}
	previews := make([]changePreview, len(records))
	for i, record := range records {
		found := slices.Index(v.records, record)
		if found >= 0 {
			previews[i] = v.previews[found]
		} else {
			previews[i] = previewChange(record.Change)
		}
	}
	v.records, v.previews, v.sessionID = records, previews, state.ID
	v.selected, v.window = 0, 0
	for i, record := range records {
		if record.ID == selectedID {
			v.selected = i
		}
	}
	v.Resize(v.width, v.height)
}

func (v *ChangeViewer) Resize(width, height int) {
	v.width, v.height = width, height
	innerWidth, bodyHeight := OverlayBodySize(width, height)
	if width >= 90 {
		innerWidth -= v.listWidth() + 3
	}
	v.Details.SetWidth(max(1, innerWidth))
	v.Details.SetHeight(max(1, bodyHeight-3))
	v.renderDetail()
}

func (v ChangeViewer) listWidth() int { return min(34, max(24, v.width/3)) }

func (v *ChangeViewer) selectRecord(index int) {
	if len(v.records) == 0 {
		return
	}
	index = max(0, min(len(v.records)-1, index))
	if index != v.selected {
		v.selected = index
		v.Details.GotoTop()
		v.Details.SetXOffset(0)
		v.renderDetail()
	}
}

func (v *ChangeViewer) Update(msg tea.Msg, zones *zone.Manager, prefix string) ControlIntent {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			return "close"
		case "tab", "shift+tab", "enter":
			v.detailFocus = !v.detailFocus
			return ""
		case "[":
			v.selectRecord(v.selected - 1)
			return ""
		case "]":
			v.selectRecord(v.selected + 1)
			return ""
		case "up", "down":
			if !v.detailFocus {
				delta := 1
				if k.String() == "up" {
					delta = -1
				}
				v.selectRecord(v.selected + delta)
				return ""
			}
		case "pgup", "pgdown", "home", "end", "left", "right":
			v.detailFocus = true
			if k.String() == "home" {
				v.Details.GotoTop()
				return ""
			}
			if k.String() == "end" {
				v.Details.GotoBottom()
				return ""
			}
		}
	}
	if mouse, ok := msg.(tea.MouseClickMsg); ok && mouse.Button == tea.MouseLeft {
		if zones.Get(prefix + "close").InBounds(mouse) {
			return "close"
		}
		if zones.Get(prefix + "detail").InBounds(mouse) {
			v.detailFocus = true
		}
		for i := range v.records {
			if zones.Get(fmt.Sprintf("%schange-%d", prefix, i)).InBounds(mouse) {
				v.selectRecord(i)
				v.detailFocus = v.width < 90
				return ""
			}
		}
	}
	if _, ok := msg.(tea.MouseWheelMsg); ok {
		v.detailFocus = true
	}
	if v.detailFocus {
		v.Details, _ = v.Details.Update(msg)
	}
	return ""
}

func (v *ChangeViewer) renderDetail() {
	v.Details.Style = lipgloss.NewStyle()
	if !v.Theme.Monochrome {
		p := v.Theme.palette()
		v.Details.Style = v.Details.Style.Foreground(lipgloss.Color(p.Text)).Background(lipgloss.Color(p.Background))
	}
	if len(v.records) == 0 {
		v.Details.SetContent("")
		return
	}
	c := v.records[v.selected].Change
	content := v.previews[v.selected].render(v.Theme)
	if c.Unavailable != "" {
		content = v.Theme.Muted(ansi.Wrap(v.Theme.T("changes."+c.Unavailable), max(1, v.Details.Width()), ""))
	} else if content == "" {
		content = v.Theme.Muted(v.Theme.T("changes.emptyFile"))
	}
	offset := v.Details.YOffset()
	v.Details.SetContent(content)
	v.Details.SetYOffset(offset)
}

func (v *ChangeViewer) View(zones *zone.Manager, prefix string, width, height int) string {
	v.Resize(width, height)
	w, h := OverlayBodySize(width, height)
	footer := zones.Mark(prefix+"close", "["+v.Theme.T("common.close")+"]")
	if len(v.records) == 0 {
		body := v.Theme.Heading(v.Theme.T("changes.empty")) + "\n\n" + v.Theme.Muted(v.Theme.T("changes.emptyHint"))
		return v.Theme.Overlay(v.Theme.T("changes.title"), v.Theme.T("changes.subtitle"), body, footer, width, height)
	}
	listWidth := w
	if width >= 90 {
		listWidth = v.listWidth()
	}
	pageSize := max(1, h-1)
	if v.selected < v.window {
		v.window = v.selected
	}
	if v.selected >= v.window+pageSize {
		v.window = v.selected - pageSize + 1
	}
	list := []string{v.Theme.Accent(v.Theme.Tf("changes.recordCount", len(v.records)))}
	for i := v.window; i < min(len(v.records), v.window+pageSize); i++ {
		c, p := v.records[i].Change, v.previews[i]
		kind := "M"
		if c.Created {
			kind = "A"
		}
		stats := fmt.Sprintf("+%d -%d", p.Added, p.Removed)
		if c.Unavailable != "" {
			stats = "…"
		}
		label := kind + " " + singleLine(string(c.Path))
		labelWidth := max(1, listWidth-ansi.StringWidth(stats)-4)
		label = ansi.Truncate(label, labelWidth, "…")
		line := "  " + label + strings.Repeat(" ", max(1, listWidth-2-ansi.StringWidth(label)-ansi.StringWidth(stats))) + v.Theme.Muted(stats)
		if i == v.selected {
			line = "> " + label + strings.Repeat(" ", max(1, listWidth-2-ansi.StringWidth(label)-ansi.StringWidth(stats))) + stats
			if !v.Theme.Monochrome {
				line = v.Theme.Selected(line)
			}
		}
		list = append(list, zones.Mark(fmt.Sprintf("%schange-%d", prefix, i), line))
	}
	for len(list) < h {
		list = append(list, "")
	}
	c, preview := v.records[v.selected].Change, v.previews[v.selected]
	detailWidth := v.Details.Width()
	path := v.Theme.reviewLine(ansi.Truncate(singleLine(string(c.Path)), detailWidth, "…"), v.Theme.palette().Accent)
	stats := v.Theme.Tf("changes.revision", len(v.records)-v.selected, len(v.records))
	if c.Unavailable == "" {
		stats += "  " + v.Theme.changeColor(fmt.Sprintf("+%d", preview.Added), true) + " " + v.Theme.changeColor(fmt.Sprintf("-%d", preview.Removed), false)
	}
	detail := path + "\n" + v.Theme.reviewLine(ansi.Truncate(stats, detailWidth, "…"), v.Theme.palette().Text) + "\n" + v.Theme.reviewLine(v.Theme.T("changes.columns"), v.Theme.palette().Muted) + "\n" + v.Details.View()
	if !v.Theme.Monochrome {
		detail = lipgloss.NewStyle().Width(detailWidth).Height(h).Background(lipgloss.Color(v.Theme.palette().Background)).Render(detail)
	}
	detail = zones.Mark(prefix+"detail", detail)
	body := strings.Join(list, "\n")
	if width >= 90 {
		divider := v.Theme.Muted(strings.TrimSuffix(strings.Repeat(" │ \n", h), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listWidth).Height(h).Render(body), divider, detail)
	} else if v.detailFocus {
		body = detail
	}
	hint := v.Theme.T("changes.listHint")
	if v.detailFocus {
		hint = v.Theme.T("changes.detailHint")
	}
	footer += "  " + v.Theme.Tf("changes.position", v.selected+1, len(v.records))
	return v.Theme.Overlay(v.Theme.T("changes.title"), hint, body, footer, width, height)
}

func (t Theme) changeColor(text string, added bool) string {
	if t.Monochrome {
		return text
	}
	color := t.palette().DiffRemoved
	if added {
		color = t.palette().Good
	}
	return t.reviewLine(text, color)
}

func (t Theme) reviewLine(text, color string) string {
	if t.Monochrome {
		return text
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Background(lipgloss.Color(t.palette().Background)).Render(text)
}
