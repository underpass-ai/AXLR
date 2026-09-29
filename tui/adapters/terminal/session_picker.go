package terminal

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
)

type SessionPicker struct {
	Items    []domain.SessionSummary
	Selected int
}

func (p SessionPicker) View(z *zone.Manager, prefix string, height, width int) string {
	var b strings.Builder
	b.WriteString("Saved sessions — local user data\n↑↓ Select • Enter Open\n")
	start := max(0, p.Selected-max(1, height-5)+1)
	end := min(len(p.Items), start+max(1, height-5))
	for i := start; i < end; i++ {
		s := p.Items[i]
		marker := "  "
		if i == p.Selected {
			marker = "> "
		}
		b.WriteString(z.Mark(fmt.Sprintf("%ssession-%d", prefix, i), ansi.Truncate(marker+singleLine(fmt.Sprintf("%s | %s | %s", s.ID, s.Model, s.Workspace)), width, "…")) + "\n")
	}
	if len(p.Items) == 0 {
		b.WriteString("No saved sessions\n")
	}
	b.WriteString(z.Mark(prefix+"close", "[Close Esc]"))
	return b.String()
}
