package anthropic

// messageDTO is one message of the conversation. Content holds blockDTO
// values and, for an assistant's thinking, the raw blocks the API returned,
// so a signature goes back byte for byte.
type messageDTO struct {
	Role    string `json:"role"`
	Content []any  `json:"content"`
}
