package application

import "github.com/underpass-ai/AXLR/tui/domain"

// correlatedDiagnostics stamps the execution's correlation on every record
// that passes through it, so the loop's trace can be joined to the task
// the record belongs to without the loop naming it at each record.
type correlatedDiagnostics struct {
	next        DiagnosticPort
	correlation DiagnosticCorrelation
}

func (d correlatedDiagnostics) Record(event DiagnosticEvent) error {
	if event.Correlation == nil {
		correlation := d.correlation
		event.Correlation = &correlation
	}
	return d.next.Record(event)
}

// CorrelatedDiagnostics is the port with the execution's correlation on
// every record; the port itself without one, or without a next.
func CorrelatedDiagnostics(next DiagnosticPort, correlation domain.ExecutionCorrelation) DiagnosticPort {
	if next == nil || correlation.IsZero() {
		return next
	}
	return correlatedDiagnostics{next: next, correlation: DiagnosticCorrelation{BotID: correlation.BotID, TaskID: correlation.TaskID, ExecutionID: correlation.ExecutionID, TraceID: correlation.TraceID}}
}

// sessionDiagnostics is the port with the session's correlation, when it
// has one.
func sessionDiagnostics(next DiagnosticPort, s domain.Session) DiagnosticPort {
	correlation, has := s.Correlation()
	if !has {
		return next
	}
	return CorrelatedDiagnostics(next, correlation)
}
