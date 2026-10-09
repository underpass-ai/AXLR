package openrouter

import "encoding/json"

type responseDTO struct {
	Choices []choiceDTO `json:"choices"`
	Usage   *usageDTO   `json:"usage"`
	// Provider is the upstream OpenRouter routed the request to.
	Provider string `json:"provider"`
	// Error is what OpenRouter sends with HTTP 200 for a failure found after
	// it accepted the request.
	Error json.RawMessage `json:"error"`
}
