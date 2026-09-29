package application

import (
	"context"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ToolCatalogPort interface {
	Snapshot(context.Context) ([]domain.AvailableTool, error)
}
