package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// MemoryAgentsPort keeps the agent the console registered in each memory
// store.
type MemoryAgentsPort interface {
	Agent(ctx context.Context, store string) (domain.MemoryAgentID, bool)
	Remember(ctx context.Context, store string, agentID domain.MemoryAgentID, registrationKey string) error
}
