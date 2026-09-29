package application

import (
	"context"

	"github.com/underpass-ai/AXLR/domain"
)

type CallPluginToolUseCase struct{ Plugins PluginToolPort }

func (u CallPluginToolUseCase) Execute(ctx context.Context, call domain.PluginCall) (domain.PluginResult, error) {
	return u.Plugins.Call(ctx, call)
}
