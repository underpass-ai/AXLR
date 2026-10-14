package application

import "github.com/underpass-ai/AXLR/tui/domain"

// GuideResult is what the memory engine gave the console when it
// registered: the agent and the context it opened.
type GuideResult struct {
	AgentID   domain.MemoryAgentID
	ContextID domain.MemoryContextID
}
