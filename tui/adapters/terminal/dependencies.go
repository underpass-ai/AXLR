package terminal

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Dependencies struct {
	Context      context.Context
	Models       application.ListModelsUseCase
	Create       application.CreateSessionUseCase
	Change       application.ChangeSessionModelUseCase
	Workspace    domain.Workspace
	NewSessionID domain.SessionID
	Start        application.StartTurnUseCase
	Resolve      application.ResolveToolUseCase
	Agent        application.AgentTurnUseCase
	Search       application.SearchSessionUseCase
	Store        application.SessionStorePort
	Session      *domain.Session
	Monochrome   bool
}
