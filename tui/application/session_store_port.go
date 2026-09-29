package application

import (
	"context"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type SessionStorePort interface {
	Save(context.Context, domain.Session) error
	Load(context.Context, domain.SessionID) (domain.Session, error)
}
