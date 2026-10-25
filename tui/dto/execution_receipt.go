package dto

import "time"

// ExecutionReceiptSchema names the wire form of an execution's receipt; a
// reader checks it before the fields.
const ExecutionReceiptSchema = "axlr.execution-receipt/v1"

// ExecutionReceipt is the runtime contract's receipt (docs/runtime-contract.md):
// what `axlr-tui --exec --json` writes before the result and what the
// service shows in a session.
type ExecutionReceipt struct {
	Schema      string                `json:"schema"`
	Status      string                `json:"status"`
	Session     string                `json:"session_id"`
	Sessions    []string              `json:"sessions"`
	StartedAt   time.Time             `json:"started_at"`
	EndedAt     time.Time             `json:"ended_at"`
	Policy      *ReceiptPolicy        `json:"policy,omitempty"`
	Budget      *ReceiptBudget        `json:"budget,omitempty"`
	Correlation *ExecutionCorrelation `json:"correlation,omitempty"`
	Requests    []ReceiptRequest      `json:"requests"`
	Calls       []ReceiptCall         `json:"tool_calls"`
	Total       ExecutionUsage        `json:"total"`
	Complete    bool                  `json:"complete"`
}
