package dto

// ExecutionPolicyMemory is whom a policy shares the execution's memory
// with (docs/runtime-contract.md, "Policy per execution").
type ExecutionPolicyMemory struct {
	Team string `json:"team"`
}
