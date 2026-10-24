package dto

import "encoding/json"

// ExecutionResultSchema names the wire form of an execution's result; a
// reader checks it before the fields.
const ExecutionResultSchema = "axlr.execution-result/v1"

// ExecutionResult is the runtime contract's result object
// (docs/runtime-contract.md): what `axlr-tui --exec --json` writes last and
// what the service puts in a session and its terminal events.
type ExecutionResult struct {
	Schema   string          `json:"schema"`
	Status   string          `json:"status"`
	Session  string          `json:"session_id"`
	Sessions []string        `json:"sessions"`
	Answer   string          `json:"answer"`
	Pending  *ExecutionCall  `json:"pending,omitempty"`
	Error    string          `json:"error,omitempty"`
	Calls    []ExecutionCall `json:"tool_calls"`
	Usage    ExecutionUsage  `json:"usage"`
	// Correlation repeats the execution's ids; absent without any.
	Correlation *ExecutionCorrelation `json:"correlation,omitempty"`
	// Artifacts is reserved for the deliverables an execution hands over
	// (Phase 1 of the master plan); empty until then.
	Artifacts []json.RawMessage `json:"artifacts"`
}
