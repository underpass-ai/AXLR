package storage

import (
	"fmt"

	"github.com/underpass-ai/AXLR/tui/adapters/contract"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ExecutionRouting is the settings' default routing for an execution, the
// contract's object (docs/runtime-contract.md, "Routing per execution");
// nil without one.
func (s UserSettings) ExecutionRouting() (*domain.ExecutionRouting, error) {
	if len(s.Routing) == 0 || string(s.Routing) == "null" {
		return nil, nil
	}
	routing, err := contract.ParseExecutionRouting(s.Routing)
	if err != nil {
		return nil, fmt.Errorf("settings routing: %w", err)
	}
	return &routing, nil
}
