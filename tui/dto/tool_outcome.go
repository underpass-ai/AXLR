package dto

type ToolOutcome struct {
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
	Uncertain bool   `json:"uncertain"`
}
