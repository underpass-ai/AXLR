package dto

import "encoding/json"

type AvailableTool struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Parameters     json.RawMessage `json:"parameters"`
	Kind           string          `json:"kind"`
	LocalOperation string          `json:"local_operation"`
	PluginID       string          `json:"plugin_id"`
	PluginToolName string          `json:"plugin_tool_name"`
}
