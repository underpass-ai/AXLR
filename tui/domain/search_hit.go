package domain

import axlr "github.com/underpass-ai/AXLR/domain"

// SearchHit identifies a matching text block. Draft hits have no message index.
type SearchHit struct {
	MessageIndex       *int
	ArchivedDraftIndex *int
	Draft              bool
	Content            axlr.Text
}
