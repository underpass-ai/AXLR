package openai

import "encoding/json"

// toolDTO is a function tool: flat, not nested under "function" as in the
// chat completions shape.
type toolDTO struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}
