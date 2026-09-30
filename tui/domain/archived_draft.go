package domain

import axlr "github.com/underpass-ai/AXLR/domain"

// ArchivedDraft keeps a partial response in the visible conversation without
// adding it to the model's message history. AfterMessage is one-based.
type ArchivedDraft struct {
	AfterMessage int
	Content      axlr.Text
}
