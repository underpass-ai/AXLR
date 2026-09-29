package openrouter

type responseDTO struct {
	Choices []choiceDTO `json:"choices"`
	Usage   *usageDTO   `json:"usage"`
}
