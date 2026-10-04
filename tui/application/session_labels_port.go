package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// SessionLabelsPort stores titles and archive flags for saved sessions.
type SessionLabelsPort interface {
	Load(context.Context) (map[domain.SessionID]domain.SessionLabel, error)
	// Set replaces title/archive metadata, retaining an established about when
	// omitted. An empty label removes the entry only if it has no about.
	Set(context.Context, domain.SessionID, domain.SessionLabel) error
	// Initialize fills only missing title/about fields atomically, preserving
	// user titles, existing scopes and archive flags. Empty fields are ignored.
	Initialize(context.Context, domain.SessionID, domain.SessionLabel) (domain.SessionLabel, error)
}
