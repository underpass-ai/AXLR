package domain

import (
	"errors"
	"fmt"
	"math"

	root "github.com/underpass-ai/AXLR/domain"
)

// ExecutionRouting chooses the model of each request of one execution
// (docs/runtime-contract.md, "Routing per execution"): the candidates it
// may ask, the objective that ranks them, and the constraints that
// exclude a candidate outright, whatever its rank: a model off this host
// when remote providers are not allowed, a model whose cap the request
// exceeds, a request expected to cost more than its ceiling. Each
// constraint is zero, or absent, for none; remote providers are allowed
// unless the routing says otherwise.
type ExecutionRouting struct {
	candidates       []root.ModelID
	objective        RoutingObjective
	allowRemote      bool
	maxContextTokens int
	maxRequestUSD    float64
}

// Bounds of a routing.
const (
	MaxRoutingCandidates    = 8
	MaxRoutingContextTokens = 16 << 20
	MaxRoutingRequestUSD    = 1000
)

func NewExecutionRouting(candidates []root.ModelID, objective RoutingObjective, allowRemote bool, maxContextTokens int, maxRequestUSD float64) (ExecutionRouting, error) {
	routing := ExecutionRouting{candidates: append([]root.ModelID(nil), candidates...), objective: objective, allowRemote: allowRemote, maxContextTokens: maxContextTokens, maxRequestUSD: maxRequestUSD}
	return routing, routing.Validate()
}

func (r ExecutionRouting) Validate() error {
	if len(r.candidates) == 0 || len(r.candidates) > MaxRoutingCandidates {
		return fmt.Errorf("a routing names between 1 and %d candidate models", MaxRoutingCandidates)
	}
	seen := make(map[root.ModelID]bool, len(r.candidates))
	for _, candidate := range r.candidates {
		if _, err := root.NewModelID(string(candidate)); err != nil {
			return fmt.Errorf("a routing's candidate %q: %w", candidate, err)
		}
		if seen[candidate] {
			return fmt.Errorf("a routing names the candidate %s twice", candidate)
		}
		seen[candidate] = true
	}
	if err := r.objective.Validate(); err != nil {
		return err
	}
	switch {
	case r.maxContextTokens < 0 || r.maxContextTokens > MaxRoutingContextTokens:
		return fmt.Errorf("a routing's max_context_tokens is between 0 and %d", MaxRoutingContextTokens)
	case math.IsNaN(r.maxRequestUSD) || math.IsInf(r.maxRequestUSD, 0) || r.maxRequestUSD < 0:
		return errors.New("a routing's max_request_usd must be a non-negative number")
	case r.maxRequestUSD > MaxRoutingRequestUSD:
		return fmt.Errorf("a routing's max_request_usd exceeds %d", MaxRoutingRequestUSD)
	}
	return nil
}

// Candidates are the models the execution may ask, in the order given:
// for quality-first, best first.
func (r ExecutionRouting) Candidates() []root.ModelID {
	return append([]root.ModelID(nil), r.candidates...)
}

func (r ExecutionRouting) Objective() RoutingObjective { return r.objective }
func (r ExecutionRouting) AllowsRemote() bool          { return r.allowRemote }
func (r ExecutionRouting) MaxContextTokens() int       { return r.maxContextTokens }
func (r ExecutionRouting) MaxRequestUSD() float64      { return r.maxRequestUSD }

// Names reports whether model is one of the candidates.
func (r ExecutionRouting) Names(model root.ModelID) bool {
	for _, candidate := range r.candidates {
		if candidate == model {
			return true
		}
	}
	return false
}

// Cap is a request's context cap for a model whose own cap is modelCap:
// narrowed by max_context_tokens; zero, unknown, when neither says.
func (r ExecutionRouting) Cap(modelCap int) int {
	if r.maxContextTokens > 0 && (modelCap <= 0 || r.maxContextTokens < modelCap) {
		return r.maxContextTokens
	}
	return modelCap
}

// Clone copies the candidates, so a caller cannot change the session's.
func (r ExecutionRouting) Clone() ExecutionRouting {
	r.candidates = append([]root.ModelID(nil), r.candidates...)
	return r
}
