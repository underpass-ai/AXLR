package application

import "github.com/underpass-ai/AXLR/tui/domain"

// DiagnosticCorrelation is the execution's correlation on a trace record
// (docs/runtime-contract.md): the ids as the control plane gave them,
// each validated safe ASCII of bounded length, never free text.
type DiagnosticCorrelation struct {
	BotID       string `json:"bot_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
}

// Valid accepts safe ids with at least one of them given.
func (c DiagnosticCorrelation) Valid() bool {
	correlation := domain.ExecutionCorrelation{BotID: c.BotID, TaskID: c.TaskID, ExecutionID: c.ExecutionID, TraceID: c.TraceID}
	return correlation.Validate() == nil && !correlation.IsZero()
}
