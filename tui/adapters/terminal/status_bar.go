package terminal

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type StatusBar struct {
	State domain.SessionStatus
	Error string
}

func (s StatusBar) View(w int) string {
	text := "Status: " + string(s.State)
	if s.Error != "" {
		text += " | Error: " + singleLine(s.Error)
	}
	return ansi.Truncate(text, w, "…")
}
