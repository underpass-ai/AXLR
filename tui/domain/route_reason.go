package domain

// RouteReason says why a request went to its model.
type RouteReason string

const (
	// RouteRanked: the routing ranked the model first for the request.
	RouteRanked RouteReason = "ranked"
	// RouteFallback: the model ranked first failed the request, and this
	// one, the next, took it again.
	RouteFallback RouteReason = "fallback"
)
