package application

import (
	"context"
	"errors"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type StartTurnUseCase struct {
	Catalog  ToolCatalogPort
	Store    SessionStorePort
	Continue ContinueTurnUseCase
}

func (u StartTurnUseCase) Execute(ctx context.Context, session *domain.Session, prompt root.Text, emit func(Event) error) error {
	if session == nil || u.Catalog == nil || u.Store == nil {
		return errors.New("start turn requires session, catalog and store")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tools, err := u.Catalog.Snapshot(ctx)
	if err != nil {
		return err
	}
	next := *session
	if err := next.BeginTurn(prompt, tools); err != nil {
		return err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*session = next
	return u.Continue.Execute(ctx, session, emit)
}
