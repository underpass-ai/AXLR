package domain

import axlr "github.com/underpass-ai/AXLR/domain"

// ToolFailureStreak is how a turn's last answers ended: Count answers in a
// row, the newest first, whose every call of Tool failed alike
// (Session.ToolFailureStreak says what alike is). Failure is the newest
// one's error as the model read it.
type ToolFailureStreak struct {
	Tool    axlr.ToolName
	Failure string
	Count   int
}

// DefaultToolFailureStreak is how many answers failed alike in a row end an
// execution when settings name no other number (tool_failure_streak): the
// bot that corrected itself on 12 November 2026 did so at its second call.
const DefaultToolFailureStreak = 3
