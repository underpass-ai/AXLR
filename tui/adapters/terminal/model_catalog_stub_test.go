package terminal

import (
	"context"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type modelCatalogStub struct{ err error }

func (c modelCatalogStub) List(context.Context) ([]domain.AvailableModel, error) {
	return []domain.AvailableModel{{ID: "provider/chosen", Name: "Chosen", SupportsTools: true, TextOutput: true}}, c.err
}
