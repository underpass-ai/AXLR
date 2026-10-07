package localmodels

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Catalog merges the configured local models into the remote catalog; the
// picker sorts the result by name, and favorites keep a model on top.
// Remote is nil without an OpenRouter key. When the remote catalog fails and
// local models exist, /model still offers the local ones; the failed request
// stays in the diagnostics trace.
type Catalog struct {
	Local  []domain.AvailableModel
	Remote application.ModelCatalogPort
}

func (c Catalog) List(ctx context.Context) ([]domain.AvailableModel, error) {
	models := append([]domain.AvailableModel(nil), c.Local...)
	if c.Remote == nil {
		return models, nil
	}
	remote, err := c.Remote.List(ctx)
	if err != nil {
		if len(models) == 0 || ctx.Err() != nil {
			return nil, err
		}
		return models, nil
	}
	return append(models, remote...), nil
}
