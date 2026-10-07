package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ModelContextWindowPort reports the context window, in tokens, the console
// must respect for a model: the configured window of a local model, capped by
// the settings' global limit. Zero means unknown, which keeps the default
// byte budget.
type ModelContextWindowPort interface {
	ContextWindow(root.ModelID) domain.ContextWindow
}
