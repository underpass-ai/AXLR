package dto

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls"`
	ToolCallID string     `json:"tool_call_id"`
}
