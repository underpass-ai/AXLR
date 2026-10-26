package contract

import (
	"encoding/json"
	"errors"
	"io"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// ParseExecutionRouting reads the contract's routing object, strictly: an
// unknown field refuses it, as the service refuses unknown fields. The
// object may come wrapped as {"routing": {...}}.
func ParseExecutionRouting(raw []byte) (domain.ExecutionRouting, error) {
	if len(raw) > MaxExecutionObjectBytes {
		return domain.ExecutionRouting{}, errors.New("the routing exceeds 4 KiB")
	}
	var wrapped struct {
		Routing json.RawMessage `json:"routing"`
	}
	if decoder := strictDecoder(raw); decoder.Decode(&wrapped) == nil && wrapped.Routing != nil {
		raw = wrapped.Routing
	}
	var wire dto.ExecutionRouting
	decoder := strictDecoder(raw)
	if err := decoder.Decode(&wire); err != nil {
		return domain.ExecutionRouting{}, errors.New("the routing must be the contract's object: candidates, objective, allow_remote_providers, max_context_tokens and max_request_usd, with no other field")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ExecutionRouting{}, errors.New("trailing data after the routing")
	}
	return DomainExecutionRouting(wire)
}

// DomainExecutionRouting validates a wire routing into the domain's.
func DomainExecutionRouting(wire dto.ExecutionRouting) (domain.ExecutionRouting, error) {
	candidates := make([]root.ModelID, 0, len(wire.Candidates))
	for _, candidate := range wire.Candidates {
		candidates = append(candidates, root.ModelID(candidate))
	}
	allowRemote := wire.AllowRemoteProviders == nil || *wire.AllowRemoteProviders
	return domain.NewExecutionRouting(candidates, domain.RoutingObjective(wire.Objective), allowRemote, wire.MaxContextTokens, wire.MaxRequestUSD)
}

// ExecutionRouting is the wire form of a routing, as GetSession shows it:
// allow_remote_providers spelled out.
func ExecutionRouting(routing domain.ExecutionRouting) dto.ExecutionRouting {
	allowRemote := routing.AllowsRemote()
	wire := dto.ExecutionRouting{Candidates: []string{}, Objective: string(routing.Objective()), AllowRemoteProviders: &allowRemote, MaxContextTokens: routing.MaxContextTokens(), MaxRequestUSD: routing.MaxRequestUSD()}
	for _, candidate := range routing.Candidates() {
		wire.Candidates = append(wire.Candidates, string(candidate))
	}
	return wire
}
