package anthropic

import "encoding/json"

// blockDTO is one content block of a message, in a request or a reply: text,
// tool_use, tool_result, thinking or redacted_thinking. Thinking blocks are
// kept whole in Raw, so their signatures go back exactly as they came.
type blockDTO struct {
	Type         string           `json:"type"`
	Text         string           `json:"text,omitempty"`
	ID           string           `json:"id,omitempty"`
	Name         string           `json:"name,omitempty"`
	Input        json.RawMessage  `json:"input,omitempty"`
	ToolUseID    string           `json:"tool_use_id,omitempty"`
	Content      string           `json:"content,omitempty"`
	IsError      bool             `json:"is_error,omitempty"`
	Thinking     *string          `json:"thinking,omitempty"`
	Signature    string           `json:"signature,omitempty"`
	Data         string           `json:"data,omitempty"`
	CacheControl *cacheControlDTO `json:"cache_control,omitempty"`
}
