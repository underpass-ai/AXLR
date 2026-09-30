package terminal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type SessionPicker struct {
	Items    []domain.SessionSummary
	Selected int
}

func (p SessionPicker) View(theme Theme, z *zone.Manager, prefix string, height, width int) string {
	innerWidth, bodyHeight := OverlayBodySize(width, height)
	page := max(1, bodyHeight/3)
	start := max(0, p.Selected-page+1)
	start = min(start, max(0, len(p.Items)-page))
	end := min(len(p.Items), start+page)
	var lines []string
	for i := start; i < end; i++ {
		s := p.Items[i]
		marker := "  "
		if i == p.Selected {
			marker = "› "
		}
		label := ansi.Truncate(marker+singleLine(string(s.Model)), innerWidth, "…")
		if i == p.Selected {
			label = theme.Selected(label)
		}
		lines = append(lines, z.Mark(fmt.Sprintf("%ssession-%d", prefix, i), label))
		meta := "  " + singleLine(string(s.ID)) + " · " + singleLine(string(s.Workspace))
		lines = append(lines, theme.Muted(ansi.Truncate(meta, innerWidth, "…")), "")
	}
	if len(p.Items) == 0 {
		lines = append(lines, theme.T("sessions.empty"))
	}
	footer := z.Mark(prefix+"close", "["+theme.T("common.close")+"]")
	hint := theme.Tf("sessions.hint", len(p.Items))
	return theme.Overlay(theme.T("sessions.title"), hint, strings.Join(lines, "\n"), footer, width, height)
}
