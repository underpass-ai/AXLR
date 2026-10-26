package application

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ModelRoutingPort tells the router what the console knows of a candidate
// model before a request: whether this host serves it, whether a client
// reaches it and its context cap (Cap). The request's size, the routing's
// narrowing and the model's stats are the application's to add
// (model_routing.go). CatalogPrice is the model's price in dollars per
// prompt and completion token from the provider's catalog, false when it
// lists none: it may read the catalog over the network, so the router asks
// only when a remote candidate's cost matters and no request of it
// reported one.
type ModelRoutingPort interface {
	RouteFacts(ctx context.Context, model root.ModelID) domain.RouteCandidate
	CatalogPrice(ctx context.Context, model root.ModelID) (prompt, completion float64, known bool)
}
