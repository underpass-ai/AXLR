package terminal

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Header struct{ State domain.SessionState }

func (h Header) View(width int, theme Theme) string {
	if h.State.ID == "" {
		content := theme.Heading("AXLR") + theme.Muted(" · "+theme.T("header.chooseModel")+" · "+singleLine(string(h.State.Workspace)))
		return theme.overlayLine(ansi.Truncate(content, max(1, width-4), "…"), width, true)
	}
	content := theme.Heading("AXLR") + theme.Muted("  │  "+singleLine(string(h.State.Model))+"  │  "+singleLine(string(h.State.Workspace)))
	return theme.overlayLine(ansi.Truncate(content, max(1, width-4), "…"), width, true)
}
