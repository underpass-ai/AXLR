package anthropic

import "encoding/json"

// responseDTO is a whole reply, or the message a stream assembles.
type responseDTO struct {
	Type        string          `json:"type"`
	Role        string          `json:"role"`
	Model       string          `json:"model"`
	Content     []blockDTO      `json:"content"`
	StopReason  string          `json:"stop_reason"`
	StopDetails *stopDetailsDTO `json:"stop_details"`
	Usage       *usageDTO       `json:"usage"`
	Error       json.RawMessage `json:"error"`
}

type stopDetailsDTO struct {
	Type        string `json:"type"`
	Category    string `json:"category"`
	Explanation string `json:"explanation"`
}
