package openrouter

type messageDTO struct {
	Role       string        `json:"role"`
	Content    *string       `json:"content"`
	ToolCalls  []toolCallDTO `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}
