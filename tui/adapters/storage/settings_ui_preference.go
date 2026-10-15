package storage

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type settingsUIPreference struct{ store *UserSettingsStore }

var _ application.UIPreferencePort = settingsUIPreference{}

func (p settingsUIPreference) Load(ctx context.Context) (domain.UIPreferences, error) {
	s, err := p.store.Load(ctx)
	return s.UIPreferences(), err
}

func (p settingsUIPreference) Save(ctx context.Context, ui domain.UIPreferences) error {
	if err := ui.Validate(); err != nil {
		return err
	}
	return p.store.update(ctx, func(s *UserSettings) {
		s.Theme = string(ui.Theme)
		s.Icons = string(ui.Icons)
		s.ReduceMotion = ui.ReduceMotion
		s.NaturalWriting = ui.NaturalWriting
	})
}
