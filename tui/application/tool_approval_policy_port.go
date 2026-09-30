package application

import "github.com/underpass-ai/AXLR/tui/domain"

type ToolApprovalPolicyPort interface {
	AutoApproves(domain.ToolIdentity) bool
}
