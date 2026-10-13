package application

import (
	"context"
	"errors"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type CreateSessionUseCase struct {
	Store SessionStorePort
	// NewID draws the ID of a session ExecuteNew creates; nil draws a
	// random one.
	NewID func() (domain.SessionID, error)
	// Labels and About bind every new session of the workspace to the
	// about its committed KMP bundle names, as the person would with
	// axlr_session; nil Labels or an empty About binds nothing. Sessions
	// the console creates after launch (/new, a plan's workers) get it
	// this way, not only the launched one.
	Labels SessionLabelsPort
	About  string
}

// ExecuteNew creates and saves a session under a fresh ID, such as the one
// the person starts from the console without relaunching it.
func (u CreateSessionUseCase) ExecuteNew(ctx context.Context, workspace domain.Workspace, model root.ModelID) (domain.Session, error) {
	newID := u.NewID
	if newID == nil {
		newID = NewRandomSessionID
	}
	id, err := newID()
	if err != nil {
		return domain.Session{}, err
	}
	return u.Execute(ctx, id, workspace, model)
}

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
	if u.Labels != nil && u.About != "" {
		if _, err := u.Labels.Initialize(ctx, id, domain.SessionLabel{About: u.About}); err != nil {
			return domain.Session{}, err
		}
	}
	return session, nil
}
