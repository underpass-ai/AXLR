package storage

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

type settingsModelPreference struct{ store *UserSettingsStore }

var _ application.ModelPreferencePort = settingsModelPreference{}

func (p settingsModelPreference) Load(ctx context.Context) (root.ModelID, error) {
	s, err := p.store.Load(ctx)
	return root.ModelID(s.Model), err
}

func (p settingsModelPreference) Save(ctx context.Context, model root.ModelID) error {
	if _, err := root.NewModelID(string(model)); err != nil {
		return err
	}
	return p.store.update(ctx, func(s *UserSettings) { s.Model = string(model) })
}
