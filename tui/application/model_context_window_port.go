package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ModelContextWindowPort reports what the console knows about a model's
// context. ContextWindow is the window, in tokens, it must respect: the
// configured window of a local model, capped by the settings' global limit;
// zero means unknown. ContextBudget is the byte budget of the projection:
// derived from the window of a local model, and from the settings' prompt
// budget for a remote one, whose every prompt token is paid for.
type ModelContextWindowPort interface {
	ContextWindow(root.ModelID) domain.ContextWindow
	ContextBudget(root.ModelID) domain.ContextBudget
}

// sessionModel is the model a session's requests go to: a ceremony may ask
// another model than the session's, such as the planner.
func sessionModel(s domain.Session) root.ModelID {
	model := s.Export().Model
	if run, live := s.Ceremony(); live && run.Model != "" {
		model = root.ModelID(run.Model)
	}
	return model
}
