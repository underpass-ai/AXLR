package storage

import (
	"fmt"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// tool_failure_streak bounds: one failure is a model's mistake it can
// correct, and past 32 the turn's whole default budget goes to one loop.
const (
	minToolFailureStreak = 2
	maxToolFailureStreak = 32
)

// ExecToolFailureStreak is how many answers in a row, failed alike, end
// an --exec run: tool_failure_streak, else
// domain.DefaultToolFailureStreak.
func (s UserSettings) ExecToolFailureStreak() int {
	if s.ToolFailureStreak == 0 {
		return domain.DefaultToolFailureStreak
	}
	return s.ToolFailureStreak
}

func (s UserSettings) validateToolFailureStreak() error {
	if s.ToolFailureStreak != 0 && (s.ToolFailureStreak < minToolFailureStreak || s.ToolFailureStreak > maxToolFailureStreak) {
		return fmt.Errorf("settings tool_failure_streak must be between %d and %d", minToolFailureStreak, maxToolFailureStreak)
	}
	return nil
}
