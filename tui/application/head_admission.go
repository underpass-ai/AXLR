package application

import "github.com/underpass-ai/AXLR/tui/domain"

// headAdmission is what rejectUnknown checks the call at the head against
// before any card or automatic approval.
type headAdmission struct {
	Store SessionStorePort
	Trace DiagnosticPort
	// Validation checks local arguments against their schema; nil leaves
	// that to the runtime.
	Validation ToolArgumentValidationPort
	// Forge checks forged-tool calls before their card; nil leaves that to
	// the host tool.
	Forge ForgedToolsPort
	// Publisher checks axlr_publish_tool before its card; nil leaves that
	// to the host tool.
	Publisher ToolPublisherPort
	// TurnLimit is the turn's tool-call budget; zero means
	// domain.MaxTurnToolCalls.
	TurnLimit int
}

func (a headAdmission) turnLimit() int {
	if a.TurnLimit > 0 {
		return a.TurnLimit
	}
	return domain.MaxTurnToolCalls
}
