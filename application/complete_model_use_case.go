package application

import (
	"context"
	"errors"

	"github.com/underpass-ai/AXLR/domain"
)

type CompleteModelUseCase struct{ Models ModelPort }

func (u CompleteModelUseCase) Execute(ctx context.Context, req domain.CompletionRequest) (domain.CompletionResult, error) {
	if err := req.Validate(); err != nil {
		return domain.CompletionResult{}, err
	}
	if u.Models == nil {
		return domain.CompletionResult{}, errors.New("model port is nil")
	}
	return u.Models.Complete(ctx, req)
}
