package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

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
	// Approval is the policy of the use case driving the turn; a side
	// effect it approves without a card runs without a why (whyError).
	// nil refuses every call without one.
	Approval ToolApprovalPolicyPort
}

// whyError refuses a side effect without a why, unless the policy approves
// the call without a card: nobody reads that why, and the refusal made the
// model send the whole call again. On 22 October 2026 claude-haiku-5.5,
// running headless with local calls approved, forgot the why on five writes and
// edits and resent about 30,000 output tokens of HTML each time. The call
// runs, and its result asks the model for a why next time (whyReminder).
// checked is the tool the later schema checks use: without the why
// requirement when the missing why was let through.
func (a headAdmission) whyError(s domain.Session, tool domain.AvailableTool, arguments root.JSONValue) (checked domain.AvailableTool, err error) {
	err = whyError(tool, arguments)
	if err != nil && CallWhy(arguments) == "" && a.Approval != nil && approvesInSession(a.Approval, s, tool.Identity, arguments) {
		return whyOptional(tool), nil
	}
	return tool, err
}

func (a headAdmission) turnLimit() int {
	if a.TurnLimit > 0 {
		return a.TurnLimit
	}
	return domain.MaxTurnToolCalls
}
