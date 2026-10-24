package dto

// ExecutionBudget is the runtime contract's budget per execution
// (docs/runtime-contract.md): what `axlr-tui --exec --budget` reads and
// what CreateSession takes and GetSession shows. A zero or absent limit
// is no limit.
type ExecutionBudget struct {
	Seconds   int     `json:"seconds,omitempty"`
	Requests  int     `json:"requests,omitempty"`
	ToolCalls int     `json:"tool_calls,omitempty"`
	USD       float64 `json:"usd,omitempty"`
}
