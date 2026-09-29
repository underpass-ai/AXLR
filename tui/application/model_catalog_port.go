package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

type ModelCatalogPort interface {
	List(context.Context) ([]domain.AvailableModel, error)
}
