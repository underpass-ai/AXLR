package application

import (
	"context"
	"errors"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type CreateSessionUseCase struct{ Store SessionStorePort }

func (u CreateSessionUseCase) Execute(ctx context.Context, id domain.SessionID, workspace domain.Workspace, model root.ModelID) (domain.Session, error) {
	if u.Store == nil {
		return domain.Session{}, errors.New("create session requires store")
	}
	if err := ctx.Err(); err != nil {
		return domain.Session{}, err
	}
	session, err := domain.NewSession(id, workspace, model)
	if err != nil {
		return domain.Session{}, err
	}
	if err := u.Store.Save(ctx, session); err != nil {
		return domain.Session{}, err
	}
	return session, nil
}
