package application

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
)

// TokenCalibrationPort learns how many bytes a model's prompt tokens hold:
// each request reports the size of the body sent and the prompt tokens the
// provider counted for it.
type TokenCalibrationPort interface {
	Observe(ctx context.Context, model root.ModelID, requestBytes, promptTokens int) error
}
