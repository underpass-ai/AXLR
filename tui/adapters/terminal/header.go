package terminal

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Header struct{ State domain.SessionState }

func (h Header) View(width int, theme Theme) string {
	if h.State.ID == "" {
		const prefix = "AXLR | "
		const guidance = " | Type /model to choose a model"
		workspaceWidth := max(0, width-ansi.StringWidth(prefix+guidance))
		workspace := ansi.Truncate(singleLine(string(h.State.Workspace)), workspaceWidth, "…")
		return theme.Heading(ansi.Truncate(prefix+workspace+guidance, width, "…"))
	}
	return theme.Heading(ansi.Truncate("AXLR | "+singleLine(string(h.State.Model))+" | "+singleLine(string(h.State.Workspace)), width, "…"))
}
