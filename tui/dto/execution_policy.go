package dto

// ExecutionPolicy is the runtime contract's policy per execution
// (docs/runtime-contract.md): what `axlr-tui --exec --policy` reads and
// what CreateSession takes and GetSession shows.
type ExecutionPolicy struct {
	Level string               `json:"level"`
	Tools ExecutionPolicyTools `json:"tools"`
	// Memory is absent when the execution's memory is its own alone.
	Memory *ExecutionPolicyMemory `json:"memory,omitempty"`
}

// ExecutionPolicyTools lists the rules of a policy; an absent allow list
// forbids nothing by name, a present one forbids every tool it omits.
type ExecutionPolicyTools struct {
	Allow []ExecutionPolicyRule `json:"allow,omitempty"`
	Deny  []ExecutionPolicyRule `json:"deny,omitempty"`
}
