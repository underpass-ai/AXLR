package application

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
)

// ModelFavoritesPort stores the models the user pins at the top of /model.
type ModelFavoritesPort interface {
	Load(context.Context) ([]root.ModelID, error)
	// Toggle adds the model when absent and removes it when present, and
	// returns the resulting list.
	Toggle(context.Context, root.ModelID) ([]root.ModelID, error)
}
