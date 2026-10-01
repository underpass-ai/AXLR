package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// SessionLabelsPort stores titles and archive flags for saved sessions.
type SessionLabelsPort interface {
	Load(context.Context) (map[domain.SessionID]domain.SessionLabel, error)
	// Set replaces one session's label; an empty label removes it.
	Set(context.Context, domain.SessionID, domain.SessionLabel) error
}
