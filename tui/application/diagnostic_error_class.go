package application

// DiagnosticErrorClass groups failures without logging error messages, which
// may include server responses, tool arguments, or local file paths.
type DiagnosticErrorClass string

const (
	DiagnosticErrorNone         DiagnosticErrorClass = ""
	DiagnosticErrorCancelled    DiagnosticErrorClass = "cancelled"
	DiagnosticErrorTimeout      DiagnosticErrorClass = "timeout"
	DiagnosticErrorProvider     DiagnosticErrorClass = "provider"
	DiagnosticErrorStorage      DiagnosticErrorClass = "storage"
	DiagnosticErrorTool         DiagnosticErrorClass = "tool"
	DiagnosticErrorInvalidState DiagnosticErrorClass = "invalid_state"
	DiagnosticErrorInternal     DiagnosticErrorClass = "internal"
)

func (class DiagnosticErrorClass) Valid() bool {
	switch class {
	case DiagnosticErrorNone, DiagnosticErrorCancelled, DiagnosticErrorTimeout,
		DiagnosticErrorProvider, DiagnosticErrorStorage, DiagnosticErrorTool,
		DiagnosticErrorInvalidState, DiagnosticErrorInternal:
		return true
	default:
		return false
	}
}
