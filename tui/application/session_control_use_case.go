package application

import (
	"context"
	"errors"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// SessionControlUseCase persists explicit interruption and recovery actions.
type SessionControlUseCase struct {
	Store SessionStorePort
	Agent AgentTurnUseCase
}

func (u SessionControlUseCase) Cancel(ctx context.Context, s *domain.Session) error {
	if s == nil || u.Store == nil {
		return errors.New("cancel requires session and store")
	}
	next := *s
	if err := next.CancelPending(); err != nil {
		return err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*s = next
	return nil
}
func (u SessionControlUseCase) Resume(ctx context.Context, s *domain.Session, emit func(Event) error) error {
	if s == nil || u.Store == nil {
		return errors.New("resume requires session and store")
	}
	next := *s
	if next.Status() == domain.StatusInterrupted {
		if err := next.ResumeTurn(); err != nil {
			return err
		}
		if err := u.Store.Save(ctx, next); err != nil {
			return err
		}
		*s = next
	}
	return u.Agent.Execute(ctx, s, emit)
}
