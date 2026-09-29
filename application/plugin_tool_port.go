package application

import (
	"context"

	"github.com/underpass-ai/AXLR/domain"
)

type PluginToolPort interface {
	List(context.Context) ([]domain.PluginTool, error)
	Call(context.Context, domain.PluginCall) (domain.PluginResult, error)
}
