package application

import (
	"context"
	axlrDomain "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ToolExecutionPort interface {
	Execute(context.Context, domain.ToolIdentity, axlrDomain.JSONValue) (domain.ToolOutcome, error)
}
