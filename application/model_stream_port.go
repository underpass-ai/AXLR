package application

import (
	"context"

	"github.com/underpass-ai/AXLR/domain"
)

type ModelStreamPort interface {
	Stream(context.Context, domain.CompletionRequest, func(domain.Text) error) (domain.CompletionResult, error)
}
