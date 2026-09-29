package mcpclient

import "encoding/json"

// Tool is an external tool, identified by both server and MCP tool name.
type Tool struct {
	Ref          ToolRef         `json:"ref"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}
