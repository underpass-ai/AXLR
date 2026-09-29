package diagnostics

import (
	"context"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type SessionStore struct {
	Next  application.SessionStorePort
	Trace application.DiagnosticPort
}

func (s SessionStore) Save(ctx context.Context, session domain.Session) error {
	started := time.Now()
	err := s.Next.Save(ctx, session)
	s.record(application.DiagnosticSessionSave, started, err)
	return err
}

func (s SessionStore) Load(ctx context.Context, id domain.SessionID) (domain.Session, error) {
	started := time.Now()
	session, err := s.Next.Load(ctx, id)
	s.record(application.DiagnosticSessionLoad, started, err)
	return session, err
}

func (s SessionStore) List(ctx context.Context) ([]domain.SessionSummary, error) {
	started := time.Now()
	items, err := s.Next.List(ctx)
	s.record(application.DiagnosticSessionList, started, err)
	return items, err
}

func (s SessionStore) record(stage application.DiagnosticStage, started time.Time, err error) {
	if s.Trace == nil {
		return
	}
	class := application.DiagnosticErrorNone
	if err != nil {
		class = application.DiagnosticErrorStorage
	}
	_ = s.Trace.Record(application.DiagnosticEvent{Stage: stage, ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class})
}
