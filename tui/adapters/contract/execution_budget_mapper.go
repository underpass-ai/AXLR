package contract

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// MaxExecutionObjectBytes bounds the budget or correlation object a
// surface reads: four numbers, or four ids.
const MaxExecutionObjectBytes = 4 << 10

// ParseExecutionBudget reads the contract's budget object, strictly: an
// unknown field refuses it, as the service refuses unknown fields. The
// object may come wrapped as {"budget": {...}}.
func ParseExecutionBudget(raw []byte) (domain.ExecutionBudget, error) {
	if len(raw) > MaxExecutionObjectBytes {
		return domain.ExecutionBudget{}, errors.New("the budget exceeds 4 KiB")
	}
	var wrapped struct {
		Budget json.RawMessage `json:"budget"`
	}
	if decoder := strictDecoder(raw); decoder.Decode(&wrapped) == nil && wrapped.Budget != nil {
		raw = wrapped.Budget
	}
	var wire dto.ExecutionBudget
	decoder := strictDecoder(raw)
	if err := decoder.Decode(&wire); err != nil {
		return domain.ExecutionBudget{}, errors.New("the budget must be the contract's object: seconds, requests, tool_calls and usd, with no other field")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.ExecutionBudget{}, errors.New("trailing data after the budget")
	}
	return DomainExecutionBudget(wire)
}

// DomainExecutionBudget validates a wire budget into the domain's.
func DomainExecutionBudget(wire dto.ExecutionBudget) (domain.ExecutionBudget, error) {
	return domain.NewExecutionBudget(wire.Seconds, wire.Requests, wire.ToolCalls, wire.USD)
}

// ExecutionBudget is the wire form of a budget, as GetSession shows it.
func ExecutionBudget(budget domain.ExecutionBudget) dto.ExecutionBudget {
	return dto.ExecutionBudget{Seconds: budget.Seconds(), Requests: budget.Requests(), ToolCalls: budget.ToolCalls(), USD: budget.USD()}
}
