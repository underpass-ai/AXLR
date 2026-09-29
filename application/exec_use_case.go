package application

import (
	"context"
	"github.com/underpass-ai/AXLR/domain"
)

type ExecUseCase struct{ Processes ProcessPort }

func (u ExecUseCase) Execute(ctx context.Context, c domain.ExecCommand) (domain.ExecResult, error) {
	return u.Processes.Run(ctx, c)
}
