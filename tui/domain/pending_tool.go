package domain

import axlr "github.com/underpass-ai/AXLR/domain"

// PendingTool retains the requested call and, once resolved, its decision and outcome.
// A nil Outcome and empty Decision mean that the call awaits a decision.
type PendingTool struct {
	Call     axlr.ToolCall
	Decision ToolDecision
	Outcome  *ToolOutcome
}
