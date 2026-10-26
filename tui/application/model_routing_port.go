package application

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ModelRoutingPort tells the router what the console knows of a candidate
// model before a request: whether this host serves it, whether a client
// reaches it, its context cap (Cap) and its catalog price. The request's
// size, the routing's narrowing and the model's stats are the
// application's to add (model_routing.go).
type ModelRoutingPort interface {
	RouteFacts(ctx context.Context, model root.ModelID) domain.RouteCandidate
}
