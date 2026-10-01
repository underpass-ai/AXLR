package terminal

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// searchArea takes the composer's rows while Ctrl+F is open, so the header,
// the conversation it moves through and the footer all stay in place.
func (m AppModel) searchArea() string {
	width := max(1, m.Layout.Width)
	rows := []string{m.Theme.Muted(strings.Repeat("─", width)), ansi.Truncate(m.SearchBox.Input.View(), width, "…")}
	for len(rows) < 1+m.Composer.Input.Height() {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

func (m AppModel) searchFooter() string {
	width := max(1, m.Layout.Width)
	hints := []footerHint{
		{"next", "enter", m.Theme.T("search.nextHint")},
		{"previous", "shift+enter", m.Theme.T("search.previousHint")},
		{"close", "esc", m.Theme.T("footer.close")},
	}
	var left []string
	for _, h := range hints {
		left = append(left, m.zones.Mark(m.prefix+h.zone, m.footerKey(h.key)+m.Theme.Muted(" "+h.label)))
	}
	current := 0
	if len(m.SearchBox.Hits) > 0 {
		current = m.SearchBox.Selected + 1
	}
	right := m.Theme.Muted(fmt.Sprintf("%d/%d", current, len(m.SearchBox.Hits)))
	line := " " + strings.Join(left, "  ")
	gap := max(1, width-ansi.StringWidth(line)-ansi.StringWidth(right)-1)
	return lipgloss.NewStyle().Width(width).Render(ansi.Truncate(line+strings.Repeat(" ", gap)+right, width, "…"))
}
