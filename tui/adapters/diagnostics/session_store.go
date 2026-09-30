package diagnostics

import (
	"context"
	"errors"
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
	ctx, span := application.StartDiagnosticSpan(ctx, s.Trace, application.DiagnosticActionSessionSave, application.DiagnosticEvent{Messages: len(session.Messages())})
	err := s.Next.Save(ctx, session)
	span.End(storageErrorClass(err))
	s.record(ctx, application.DiagnosticSessionSave, started, err)
	return err
}

func (s SessionStore) Load(ctx context.Context, id domain.SessionID) (domain.Session, error) {
	started := time.Now()
	ctx, span := application.StartDiagnosticSpan(ctx, s.Trace, application.DiagnosticActionSessionLoad, application.DiagnosticEvent{})
	session, err := s.Next.Load(ctx, id)
	span.End(storageErrorClass(err))
	s.record(ctx, application.DiagnosticSessionLoad, started, err)
	return session, err
}

func (s SessionStore) List(ctx context.Context) ([]domain.SessionSummary, error) {
	started := time.Now()
	ctx, span := application.StartDiagnosticSpan(ctx, s.Trace, application.DiagnosticActionSessionList, application.DiagnosticEvent{})
	items, err := s.Next.List(ctx)
	span.End(storageErrorClass(err))
	s.record(ctx, application.DiagnosticSessionList, started, err)
	return items, err
}

func storageErrorClass(err error) application.DiagnosticErrorClass {
	if errors.Is(err, context.Canceled) {
		return application.DiagnosticErrorCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return application.DiagnosticErrorTimeout
	}
	if err != nil {
		return application.DiagnosticErrorStorage
	}
	return application.DiagnosticErrorNone
}

func (s SessionStore) record(ctx context.Context, stage application.DiagnosticStage, started time.Time, err error) {
	if s.Trace == nil {
		return
	}
	class := storageErrorClass(err)
	_ = s.Trace.Record(application.DiagnosticEvent{Stage: stage, SpanID: application.CurrentDiagnosticSpan(ctx), ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class})
}
