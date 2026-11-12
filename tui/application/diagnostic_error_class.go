package application

// DiagnosticErrorClass groups failures without logging error messages, which
// may include server responses, tool arguments, or local file paths.
type DiagnosticErrorClass string

const (
	DiagnosticErrorNone      DiagnosticErrorClass = ""
	DiagnosticErrorCancelled DiagnosticErrorClass = "cancelled"
	DiagnosticErrorTimeout   DiagnosticErrorClass = "timeout"
	DiagnosticErrorProvider  DiagnosticErrorClass = "provider"
	DiagnosticErrorStorage   DiagnosticErrorClass = "storage"
	// DiagnosticErrorTool is a call that failed in the tool's machinery:
	// the runtime or the plugin transport, or an effect left uncertain.
	DiagnosticErrorTool DiagnosticErrorClass = "tool"
	// DiagnosticErrorToolResult is a call that ran to an answer and the
	// answer is a failure: a refusal, an MCP is_error result, a command
	// that exited non-zero. Until 12 November 2026 both were "tool", and a
	// bot's ten refused memory writes read like broken plumbing.
	DiagnosticErrorToolResult   DiagnosticErrorClass = "tool_result"
	DiagnosticErrorInvalidState DiagnosticErrorClass = "invalid_state"
	DiagnosticErrorInternal     DiagnosticErrorClass = "internal"
)

func (class DiagnosticErrorClass) Valid() bool {
	switch class {
	case DiagnosticErrorNone, DiagnosticErrorCancelled, DiagnosticErrorTimeout,
		DiagnosticErrorProvider, DiagnosticErrorStorage, DiagnosticErrorTool, DiagnosticErrorToolResult,
		DiagnosticErrorInvalidState, DiagnosticErrorInternal:
		return true
	default:
		return false
	}
}
