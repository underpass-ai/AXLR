package storage

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

type settingsModelFavorites struct{ store *UserSettingsStore }

var _ application.ModelFavoritesPort = settingsModelFavorites{}

func (p settingsModelFavorites) Load(ctx context.Context) ([]root.ModelID, error) {
	s, err := p.store.Load(ctx)
	return modelIDs(s.FavoriteModels), err
}

func (p settingsModelFavorites) Toggle(ctx context.Context, model root.ModelID) ([]root.ModelID, error) {
	if _, err := root.NewModelID(string(model)); err != nil {
		return nil, err
	}
	var result []string
	err := p.store.update(ctx, func(s *UserSettings) {
		next := make([]string, 0, len(s.FavoriteModels)+1)
		removed := false
		for _, id := range s.FavoriteModels {
			if id == string(model) {
				removed = true
				continue
			}
			next = append(next, id)
		}
		if !removed && len(next) < maxFavoriteModels {
			next = append(next, string(model))
		}
		s.FavoriteModels = next
		result = next
	})
	return modelIDs(result), err
}

func modelIDs(ids []string) []root.ModelID {
	out := make([]root.ModelID, len(ids))
	for i, id := range ids {
		out[i] = root.ModelID(id)
	}
	return out
}
