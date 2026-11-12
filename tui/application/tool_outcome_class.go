package application

import "github.com/underpass-ai/AXLR/tui/domain"

// toolOutcomeClass is the trace's class for how a call ended: "tool" when
// the machinery failed (an error, an uncertain effect), "tool_result" when
// the call answered with a failure, none when it succeeded.
func toolOutcomeClass(outcome domain.ToolOutcome, runErr error) DiagnosticErrorClass {
	switch {
	case runErr != nil || outcome.Uncertain:
		return DiagnosticErrorTool
	case outcome.IsError:
		return DiagnosticErrorToolResult
	default:
		return DiagnosticErrorNone
	}
}
