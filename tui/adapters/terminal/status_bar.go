package terminal

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type StatusBar struct {
	Locale      Locale
	Phase       domain.ProviderPhase
	State       domain.SessionStatus
	Error       string
	Waiting     bool
	WaitSeconds int
	Indicator   string
	Executing   bool
	ToolName    string
	ToolSeconds int
	Autonomous  bool
}

func (s StatusBar) View(w int) string {
	state := s.State
	if state == "" {
		state = domain.StatusIdle
	}
	text := Translate(s.Locale, "status.prefix") + Translate(s.Locale, "status."+string(state))
	if s.Autonomous {
		text += " | " + Translate(s.Locale, "status.autonomous")
	}
	indicator := s.Indicator
	if indicator != "" {
		indicator += "  "
	}
	if s.Waiting {
		label := Translate(s.Locale, "status.waiting")
		switch s.Phase {
		case domain.ProviderReasoning:
			label = Translate(s.Locale, "status.reasoning")
		case domain.ProviderToolCall:
			label = Translate(s.Locale, "status.preparingTools")
		}
		text += fmt.Sprintf(" | %s%s · %ds", indicator, label, s.WaitSeconds)
	}
	if s.Executing {
		name := s.ToolName
		if name == "" {
			name = Translate(s.Locale, "common.tool")
		}
		text += " | " + indicator + Translatef(s.Locale, "status.executing", name, s.ToolSeconds)
	}
	if s.Error != "" {
		text += " | " + Translate(s.Locale, "status.error") + singleLine(s.Error)
	}
	return ansi.Truncate(text, w, "…")
}
