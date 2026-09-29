package terminal

import (
	"context"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type submissionStore struct {
	application.SessionStorePort
	err error
}

func (s submissionStore) Save(context.Context, domain.Session) error { return s.err }
