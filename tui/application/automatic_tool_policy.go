package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Read-only host operations are intrinsic. Wrappers have no intrinsic approval.
func automaticallyApproves(policy ToolApprovalPolicyPort, id domain.ToolIdentity) bool {
	if id.Kind == domain.ToolKindHost {
		return id.LocalOperation == domain.HostOperationTools || id.LocalOperation == domain.HostOperationHistory || id.LocalOperation == domain.HostOperationSkill
	}
	return policy != nil && policy.AutoApproves(id)
}

// approvesInMode adds the work mode: a call the mode puts under the user's
// decision is never approved automatically, autonomy included.
func approvesInMode(policy ToolApprovalPolicyPort, mode domain.WorkMode, id domain.ToolIdentity, arguments root.JSONValue) bool {
	if verdict, _ := mode.Judge(id, arguments); verdict != domain.VerdictAllow {
		return false
	}
	return automaticallyApproves(policy, id)
}
