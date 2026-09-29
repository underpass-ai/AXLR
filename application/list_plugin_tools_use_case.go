package application

import (
	"context"

	"github.com/underpass-ai/AXLR/domain"
)

type ListPluginToolsUseCase struct{ Plugins PluginToolPort }

func (u ListPluginToolsUseCase) Execute(ctx context.Context, _ domain.PluginListCommand) ([]domain.PluginTool, error) {
	return u.Plugins.List(ctx)
}
