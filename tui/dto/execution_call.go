package dto

import "encoding/json"

// ExecutionCall is one tool call of an execution in its result: the
// pending one carries its arguments, the others only what became of them.
type ExecutionCall struct {
	ID        string          `json:"call_id"`
	Tool      string          `json:"tool"`
	Decision  string          `json:"decision,omitempty"`
	Status    string          `json:"status,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}
