package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// SessionLister lists and loads saved sessions and gives their locks back.
type SessionLister interface {
	List(context.Context) ([]domain.SessionSummary, error)
	Load(context.Context, domain.SessionID) (domain.Session, error)
	Release(domain.SessionID) error
}
