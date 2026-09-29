package dto

import "encoding/json"

type PluginToolOutput struct {
	PluginID     string          `json:"plugin_id"`
	ToolName     string          `json:"tool_name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty"`
}
