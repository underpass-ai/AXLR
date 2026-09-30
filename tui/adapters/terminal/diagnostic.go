package terminal

import (
	"context"
	"errors"

	"github.com/underpass-ai/AXLR/tui/application"
)

func (m AppModel) record(event application.DiagnosticEvent) {
	if m.deps.Diagnostics == nil {
		return
	}
	if event.OperationID == 0 {
		event.OperationID = m.operationID
	}
	_ = m.deps.Diagnostics.Record(event)
}

func diagnosticErrorClass(err error) application.DiagnosticErrorClass {
	switch {
	case err == nil:
		return application.DiagnosticErrorNone
	case errors.Is(err, context.Canceled):
		return application.DiagnosticErrorCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return application.DiagnosticErrorTimeout
	default:
		return application.DiagnosticErrorInternal
	}
}
