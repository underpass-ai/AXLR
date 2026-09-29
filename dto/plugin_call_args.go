package dto

import "encoding/json"

type PluginCallArgs struct {
	PluginID  string          `json:"plugin_id"`
	ToolName  string          `json:"tool_name"`
	Arguments json.RawMessage `json:"arguments"`
}
