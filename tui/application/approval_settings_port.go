package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// ApprovalSettingsPort changes the persistent policy used by the active agent.
type ApprovalSettingsPort interface {
	ToolApprovalPolicyPort
	Allow(context.Context, domain.ToolIdentity) error
	SetAutonomous(context.Context, bool) error
	Autonomous() bool
	Allowed() []domain.ToolIdentity
}
