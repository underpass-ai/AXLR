package application

import "context"

// MemoryAgentsPort keeps the agent the console registered in each memory
// store.
type MemoryAgentsPort interface {
	Agent(ctx context.Context, store string) (string, bool)
	Remember(ctx context.Context, store, agentID, registrationKey string) error
}
