package terminal

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Header struct{ State domain.SessionState }

func (h Header) View(width int, theme Theme) string {
	if h.State.ID == "" {
		return theme.Heading(ansi.Truncate("AXLR | "+singleLine(string(h.State.Workspace))+" | Type /model to choose a model", width, "…"))
	}
	return theme.Heading(ansi.Truncate("AXLR | "+singleLine(string(h.State.Model))+" | "+singleLine(string(h.State.Workspace)), width, "…"))
}
