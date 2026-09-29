package openrouter

type choiceDTO struct {
	Message      messageDTO `json:"message"`
	FinishReason string     `json:"finish_reason"`
}
