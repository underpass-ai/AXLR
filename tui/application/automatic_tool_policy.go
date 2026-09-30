package application

import "github.com/underpass-ai/AXLR/tui/domain"

// Read-only host operations are intrinsic. Wrappers have no intrinsic approval.
func automaticallyApproves(policy ToolApprovalPolicyPort, id domain.ToolIdentity) bool {
	if id.Kind == domain.ToolKindHost {
		return id.LocalOperation == domain.HostOperationTools || id.LocalOperation == domain.HostOperationHistory || id.LocalOperation == domain.HostOperationSkill
	}
	return policy != nil && policy.AutoApproves(id)
}
