package domain

import "errors"

// RoutingObjective is what ranks an execution's candidate models
// (docs/runtime-contract.md, "Routing per execution").
type RoutingObjective string

const (
	// RoutingCost asks the model whose request is expected to cost least:
	// a model this host serves costs no dollars.
	RoutingCost RoutingObjective = "cost"
	// RoutingLatency asks the model expected to answer soonest.
	RoutingLatency RoutingObjective = "latency"
	// RoutingQualityFirst asks the candidates in the order given: the
	// first is the best, each next one its fallback.
	RoutingQualityFirst RoutingObjective = "quality-first"
)

func (o RoutingObjective) Validate() error {
	switch o {
	case RoutingCost, RoutingLatency, RoutingQualityFirst:
		return nil
	}
	return errors.New("a routing's objective is cost, latency or quality-first")
}
