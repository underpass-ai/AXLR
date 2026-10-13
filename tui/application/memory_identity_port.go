package application

import "context"

// MemoryIdentityPort registers the console's KMP agent and opens contexts:
// Guide with a registration key registers (or finds) the agent and opens
// its first context; with an agent id and a context key it opens a new
// context for that agent. A store without the guide is synced and the
// call tried again before an error is returned.
type MemoryIdentityPort interface {
	Guide(ctx context.Context, registrationKey, agentID, contextKey string) (GuideResult, error)
}
