package openai

import "encoding/json"

// responseDTO is a whole response, or the one a stream's terminal event
// carries.
type responseDTO struct {
	ID                string          `json:"id"`
	Status            string          `json:"status"`
	Error             json.RawMessage `json:"error"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Output []outputItemDTO `json:"output"`
	Usage  *usageDTO       `json:"usage"`
}
