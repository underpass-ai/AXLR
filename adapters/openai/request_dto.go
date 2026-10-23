package openai

import "encoding/json"

type requestDTO struct {
	Model string `json:"model"`
	// Input holds itemDTO values and, for an assistant's reasoning, the raw
	// items the API returned, replayed byte for byte.
	Input  []any     `json:"input"`
	Tools  []toolDTO `json:"tools,omitempty"`
	Stream bool      `json:"stream,omitempty"`
	// Store false keeps nothing on the server: AXLR's history is the record,
	// and the reasoning comes back encrypted to be replayed.
	Store     bool            `json:"store"`
	Include   []string        `json:"include,omitempty"`
	Reasoning json.RawMessage `json:"reasoning,omitempty"`
}
