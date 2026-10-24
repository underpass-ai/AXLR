package dto

// ExecutionCorrelation is the runtime contract's correlation per execution
// (docs/runtime-contract.md): the ids the control plane gives an
// execution, carried unchanged into its events, traces and records.
type ExecutionCorrelation struct {
	BotID       string `json:"bot_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
}
