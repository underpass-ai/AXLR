package dto

// ArchivedDraft is display history, separate from model messages.
type ArchivedDraft struct {
	AfterMessage int    `json:"after_message"`
	Content      string `json:"content"`
}
