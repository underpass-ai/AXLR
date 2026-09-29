package application

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
)

type ModelStreamPort interface {
	Stream(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error)
}
