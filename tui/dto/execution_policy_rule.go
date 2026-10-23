package dto

// ExecutionPolicyRule names a tool and, optionally, the patterns its
// arguments must match.
type ExecutionPolicyRule struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}
