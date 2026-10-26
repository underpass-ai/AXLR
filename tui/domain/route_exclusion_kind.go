package domain

// RouteExclusionKind is the constraint that kept a candidate from a
// request.
type RouteExclusionKind string

const (
	// RouteUnreachable: no client can send to the model.
	RouteUnreachable RouteExclusionKind = "unreachable"
	// RouteRemote: the model runs off this host and the routing does
	// not allow remote providers.
	RouteRemote RouteExclusionKind = "remote"
	// RouteWindow: the request exceeds the model's context cap.
	RouteWindow RouteExclusionKind = "window"
	// RouteCost: the request is expected to cost more than the routing's
	// max_request_usd, or its cost cannot be told under one.
	RouteCost RouteExclusionKind = "cost"
)
