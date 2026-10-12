package axlr

import (
	"context"
	"errors"

	rootApplication "github.com/underpass-ai/AXLR/application"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

// ModelStream keeps provider wiring behind AXLR's validated use case.
type ModelStream struct {
	UseCase rootApplication.StreamModelUseCase
	// LogFailure records a request the provider or the connection failed,
	// with the provider's message, in the app log; a cancelled request is
	// the person's doing and is not logged. Nil logs nothing. On 12 October
	// 2026 a wrong model id left nothing in axlr.log.
	LogFailure func(string)
}

var _ application.ModelStreamPort = ModelStream{}

func (m ModelStream) Stream(ctx context.Context, req root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	result, err := m.UseCase.Execute(ctx, req, emit)
	if err != nil && m.LogFailure != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
		m.LogFailure("model request to " + string(req.Model) + " failed: " + err.Error())
	}
	return result, err
}
