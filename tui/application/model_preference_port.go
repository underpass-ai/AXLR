package application

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
)

// ModelPreferencePort stores the default model for future TUI sessions.
// Load returns an empty ID when no preference has been saved.
type ModelPreferencePort interface {
	Load(context.Context) (root.ModelID, error)
	Save(context.Context, root.ModelID) error
}
