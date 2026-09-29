package dto

import "encoding/json"

type PluginCallOutput struct {
	Content           []json.RawMessage `json:"content"`
	StructuredContent json.RawMessage   `json:"structured_content,omitempty"`
	IsError           bool              `json:"is_error"`
}
