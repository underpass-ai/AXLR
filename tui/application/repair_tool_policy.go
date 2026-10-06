package application

import "github.com/underpass-ai/AXLR/tui/domain"

// RepairToolPolicy is the approval policy of a repair session. The clone is
// disposable and nobody sits in front of the session, so local tools run
// without a card when AutonomousLocal is set; the reproduction command and
// the merge keep their approvals regardless, because the ceremony driver
// decides those, not this policy. Plugin tools keep the configured policy.
type RepairToolPolicy struct {
	Next            ToolApprovalPolicyPort
	AutonomousLocal bool
}

var _ ToolApprovalPolicyPort = RepairToolPolicy{}

func (p RepairToolPolicy) AutoApproves(id domain.ToolIdentity) bool {
	if p.AutonomousLocal && id.Kind == domain.ToolKindLocal {
		return true
	}
	return p.Next != nil && p.Next.AutoApproves(id)
}
