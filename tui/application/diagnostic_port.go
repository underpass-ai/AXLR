package application

// DiagnosticPort receives optional, privacy-safe lifecycle events.
type DiagnosticPort interface {
	Record(DiagnosticEvent) error
}
