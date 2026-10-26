package domain

import root "github.com/underpass-ai/AXLR/domain"

// RouteExclusion is a candidate a constraint kept from a request, and why.
type RouteExclusion struct {
	Model  root.ModelID
	Kind   RouteExclusionKind
	Reason string
	// Tokens and Cap are the request's size and the model's cap, for a
	// window exclusion.
	Tokens int
	Cap    int
}
