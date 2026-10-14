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
)
