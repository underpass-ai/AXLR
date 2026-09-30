package application

import (
	"context"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type PluginManagementPort interface {
	List(context.Context) ([]domain.PluginState, error)
	SetApproval(context.Context, root.PluginID, domain.ApprovalMode) error
}
