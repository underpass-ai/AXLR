package axlr

import (
	"context"

	rootApplication "github.com/underpass-ai/AXLR/application"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

// ModelStream keeps provider wiring behind AXLR's validated use case.
type ModelStream struct {
	UseCase rootApplication.StreamModelUseCase
}

var _ application.ModelStreamPort = ModelStream{}

func (m ModelStream) Stream(ctx context.Context, req root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	return m.UseCase.Execute(ctx, req, emit)
}
