package terminal

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Dependencies struct {
	Context    context.Context
	Start      application.StartTurnUseCase
	Resolve    application.ResolveToolUseCase
	Agent      application.AgentTurnUseCase
	Search     application.SearchSessionUseCase
	Store      application.SessionStorePort
	Session    *domain.Session
	Monochrome bool
}
