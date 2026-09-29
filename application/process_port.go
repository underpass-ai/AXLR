package application

import (
	"context"
	"github.com/underpass-ai/AXLR/domain"
)

type ProcessPort interface {
	Run(context.Context, domain.ExecCommand) (domain.ExecResult, error)
}
