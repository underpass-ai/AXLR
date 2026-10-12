package application

import (
	"context"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type SessionStorePort interface {
	List(context.Context) ([]domain.SessionSummary, error)
	Save(context.Context, domain.Session) error
	Load(context.Context, domain.SessionID) (domain.Session, error)
}

// SessionDeletePort removes a saved session for good: its snapshot and
// every file kept beside it. It refuses a session another console has
// open.
type SessionDeletePort interface {
	Delete(context.Context, domain.SessionID) error
}
