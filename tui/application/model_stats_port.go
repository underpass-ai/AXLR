package application

import (
	"context"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ModelStatsPort keeps what each model's recent requests said, across the
// sessions and launches that made them: a rolling window per model and
// provider, bounded, fed with the same samples as the sessions' usage
// ledgers. The router ranks candidates from it.
type ModelStatsPort interface {
	ObserveModel(ctx context.Context, model root.ModelID, observation domain.ModelObservation) error
	ModelStats(ctx context.Context, models []root.ModelID, now time.Time) (map[root.ModelID]domain.ModelStats, error)
}
