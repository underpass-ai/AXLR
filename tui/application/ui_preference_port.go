package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

type UIPreferencePort interface {
	Load(context.Context) (domain.UIPreferences, error)
	Save(context.Context, domain.UIPreferences) error
}
