package terminal

import (
	"context"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type submissionCatalog struct{ err error }

func (c submissionCatalog) Snapshot(context.Context) ([]domain.AvailableTool, error) {
	return nil, c.err
}
