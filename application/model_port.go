package application

import (
	"context"

	"github.com/underpass-ai/AXLR/domain"
)

type ModelPort interface {
	Complete(context.Context, domain.CompletionRequest) (domain.CompletionResult, error)
}
