package terminal

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type StatusBar struct {
	Phase       domain.ProviderPhase
	State       domain.SessionStatus
	Error       string
	Waiting     bool
	WaitSeconds int
}

func (s StatusBar) View(w int) string {
	text := "Status: " + string(s.State)
	if s.Waiting {
		label := "Waiting for model"
		switch s.Phase {
		case domain.ProviderReasoning:
			label = "Model is reasoning"
		case domain.ProviderToolCall:
			label = "Model is preparing tools"
		}
		text += fmt.Sprintf(" | %s · %ds", label, s.WaitSeconds)
	}
	if s.Error != "" {
		text += " | Error: " + singleLine(s.Error)
	}
	return ansi.Truncate(text, w, "…")
}
