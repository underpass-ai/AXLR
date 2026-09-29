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
