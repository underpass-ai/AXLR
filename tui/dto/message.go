package dto

import "encoding/json"

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls"`
	ToolCallID string     `json:"tool_call_id"`
	// Reasoning is the provider's reasoning_details for an assistant
	// message, saved so a resumed session sends it back as the live one did.
	Reasoning json.RawMessage `json:"reasoning_details,omitempty"`
}
