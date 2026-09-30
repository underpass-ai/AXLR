package application

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// DiagnosticSpan records a correlated start/end pair, including failures.
// Parent IDs flow through context across application, HTTP and storage adapters.
type DiagnosticSpan struct {
	trace   DiagnosticPort
	event   DiagnosticEvent
	started time.Time
	once    sync.Once
}

var diagnosticSpanSequence atomic.Uint64

func CurrentDiagnosticSpan(ctx context.Context) uint64 {
	if ctx == nil {
		return 0
	}
	id, _ := ctx.Value(diagnosticSpanKey{}).(uint64)
	return id
}
func StartDiagnosticSpan(ctx context.Context, trace DiagnosticPort, action DiagnosticAction, event DiagnosticEvent) (context.Context, *DiagnosticSpan) {
	if ctx == nil {
		ctx = context.Background()
	}
	if trace == nil {
		return ctx, nil
	}
	event.Stage = DiagnosticActionStart
	event.Action = action
	event.ParentSpanID = CurrentDiagnosticSpan(ctx)
	event.SpanID = diagnosticSpanSequence.Add(1)
	event.ElapsedMicroseconds = 0
	span := &DiagnosticSpan{trace: trace, event: event, started: time.Now()}
	_ = trace.Record(event)
	return context.WithValue(ctx, diagnosticSpanKey{}, event.SpanID), span
}
func (s *DiagnosticSpan) End(class DiagnosticErrorClass) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		event := s.event
		event.Stage = DiagnosticActionEnd
		elapsed := time.Since(s.started)
		event.ElapsedMilliseconds = elapsed.Milliseconds()
		event.ElapsedMicroseconds = elapsed.Microseconds()
		event.ErrorClass = class
		_ = s.trace.Record(event)
	})
}
