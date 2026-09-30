package terminal

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type StatusBar struct {
	State       domain.SessionStatus
	Error       string
	Waiting     bool
	WaitSeconds int
}

func (s StatusBar) View(w int) string {
	text := "Status: " + string(s.State)
	if s.Waiting {
		text += fmt.Sprintf(" | Waiting for model · %ds", s.WaitSeconds)
	}
	if s.Error != "" {
		text += " | Error: " + singleLine(s.Error)
	}
	return ansi.Truncate(text, w, "…")
}
