package application

import (
	"context"
	"errors"

	"github.com/underpass-ai/AXLR/domain"
)

type StreamModelUseCase struct{ Models ModelStreamPort }

func (u StreamModelUseCase) Execute(ctx context.Context, req domain.CompletionRequest, emit func(domain.Text) error) (domain.CompletionResult, error) {
	if err := req.Validate(); err != nil {
		return domain.CompletionResult{}, err
	}
	if modelPortIsNil(u.Models) {
		return domain.CompletionResult{}, errors.New("model port is nil")
	}
	return u.Models.Stream(ctx, req, emit)
}
