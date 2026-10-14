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
	// Surface is absent for a tool the console does not own any of, and
	// in session files written before 14 October 2026.
	Surface *ToolSurface `json:"surface,omitempty"`
}
