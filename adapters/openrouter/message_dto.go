package openrouter

import "encoding/json"

type messageDTO struct {
	Role       string        `json:"role"`
	Content    *string       `json:"content"`
	ToolCalls  []toolCallDTO `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	// ReasoningDetails is the reasoning OpenRouter returned with an
	// assistant message, sent back unmodified, as its docs require for a
	// model that continues after its tool calls.
	ReasoningDetails json.RawMessage `json:"reasoning_details,omitempty"`
}
