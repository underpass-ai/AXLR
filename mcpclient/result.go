package mcpclient

import "encoding/json"

// Result retains MCP content blocks and optional structured output as JSON.
// IsError is a tool-level error reported by the server, not a transport error.
type Result struct {
	Content           []json.RawMessage `json:"content"`
	StructuredContent json.RawMessage   `json:"structured_content,omitempty"`
	IsError           bool              `json:"is_error"`
}
