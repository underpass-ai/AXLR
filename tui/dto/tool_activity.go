package dto

type ToolActivity struct {
	Call     ToolCall     `json:"call"`
	Decision string       `json:"decision"`
	Outcome  *ToolOutcome `json:"outcome"`
}
