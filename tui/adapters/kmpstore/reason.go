package kmpstore

// Reason says why a store was chosen.
type Reason string

const (
	// ReasonOperator is a KMP_MCP_DATA_DIR an operator set in mcp.json.
	ReasonOperator Reason = "operator"
	// ReasonProject is the checkout's .kernel.
	ReasonProject Reason = "project"
	// ReasonUser is the per-user default outside a checkout.
	ReasonUser Reason = "user"
	// ReasonProgram is the store the program that runs this execution
	// gave with --kmp-store: a node gives each bot its own, so a bot's
	// memory never lands in the operator's per-user store.
	ReasonProgram Reason = "program"
)
