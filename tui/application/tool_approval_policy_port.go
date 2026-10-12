package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ToolApprovalPolicyPort interface {
	AutoApproves(domain.ToolIdentity) bool
}

// CallApprovalPort is a policy that also approves exact calls of one
// session, such as a command the person allowed for the rest of the session
// from its card. A policy may implement it besides ToolApprovalPolicyPort.
// Unlike a saved rule it also approves a call the work mode puts under the
// person's decision, since the person decided on this exact call; a call
// the mode denies stays denied.
type CallApprovalPort interface {
	ApprovesCall(domain.SessionID, domain.ToolIdentity, root.JSONValue) bool
}
