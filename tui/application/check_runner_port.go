package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// CheckRunnerPort runs one command in the workspace with a time limit and no
// shell.
type CheckRunnerPort interface {
	Run(ctx context.Context, command domain.CheckCommand) (CheckResult, error)
}
