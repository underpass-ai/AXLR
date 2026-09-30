package application

import (
	"context"
	"errors"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ChangeSessionModelUseCase struct{ Store SessionStorePort }

func (u ChangeSessionModelUseCase) Execute(ctx context.Context, session *domain.Session, model root.ModelID) error {
	if session == nil || u.Store == nil {
		return errors.New("change model requires session and store")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	next := *session
	if err := next.ChangeModel(model); err != nil {
		return err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*session = next
	return nil
}
