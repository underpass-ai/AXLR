package application

import (
	"context"
	"errors"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// GenerationCostPort asks OpenRouter what one request cost, by the
// generation id its stream carried: the exact cost of a request that was
// cancelled or failed without a usage report (usage_settlement.go). One
// call is one lookup; the use case decides when to ask again.
type GenerationCostPort interface {
	GenerationCost(ctx context.Context, id string) (domain.GenerationCost, error)
}

// ErrGenerationPending is a lookup that found no stats for the request
// yet: OpenRouter has none for a while after it ends, longer when the
// provider keeps generating after a cancel.
var ErrGenerationPending = errors.New("the request's cost is not known yet")
