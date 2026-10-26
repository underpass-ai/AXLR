package domain

import root "github.com/underpass-ai/AXLR/domain"

// RouteCandidate is what the router knows of one candidate model when a
// request is to be sent: where it runs, what it can take, what it costs
// and what its recent requests said.
type RouteCandidate struct {
	Model root.ModelID
	// Local is a model this host serves: a local_models entry whose
	// server is on a loopback address. No request to it leaves the host.
	Local bool
	// Reachable is false for a model no client can send to: not a
	// local_models entry, and no OpenRouter key.
	Reachable bool
	// Cap is the request's context cap for this model, in tokens, as the
	// context cap applies it (zero, unknown); Tokens is the request's size
	// as this model is estimated to count it, zero when not counted.
	Cap    int
	Tokens int
	// PromptRate and CompletionRate are its price in dollars per token,
	// when RatesKnown: OpenRouter's catalog.
	PromptRate     float64
	CompletionRate float64
	RatesKnown     bool
	Stats          ModelStats
}
