package domain

import axlr "github.com/underpass-ai/AXLR/domain"

// ToolFailureStreak is how a turn's last tool calls ended: Count calls in
// a row, the newest first, of the same tool that failed alike. Failure is
// the newest one's error as the model read it; alike means the same text
// once the call's id and the values of its own arguments are taken out, so
// a model that sends the refused call again with another invented value
// (project:dev.void, project:dev.void2…) repeats the same failure.
type ToolFailureStreak struct {
	Tool    axlr.ToolName
	Failure string
	Count   int
}

// DefaultToolFailureStreak is how many alike failures in a row end an
// execution when settings name no other number (tool_failure_streak): the
// bot that corrected itself on 12 November 2026 did so at its second call.
const DefaultToolFailureStreak = 3
