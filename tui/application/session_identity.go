package application

import (
	"context"
	"encoding/json"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The console's identity in the memory engine (decision D3 of the
// lifecycle integration design): one agent per store, registered with the
// install's key, and one context per session, opened at its first prompt
// and passed by the console on every memory call the session makes. The
// model never registers, never sees an id and never opens a context.

// contextKey names the session's context; the same key reopens the same
// context when the sidecar was lost.
func contextKey(id domain.SessionID) string { return "axlr:session:" + string(id) }

// openContext opens the session's memory context when it has none: the store's
// agent is registered first when the registry holds none, and registered
// again when the engine no longer knows the recorded one (the store was emptied or
// replaced). A failure is logged and leaves the session's calls without a
// context until the next prompt.
func (c *SessionCeremony) openContext(ctx context.Context, s *domain.Session) {
	if !c.Identity.usable() {
		return
	}
	instance, _ := s.Instance()
	if instance.KMPContext != "" {
		return
	}
	id := s.ID()
	agent, known := c.Identity.Agents.Agent(ctx, c.Identity.Store)
	var result GuideResult
	var err error
	if known {
		result, err = c.Identity.Guide.Guide(ctx, "", agent, contextKey(id))
		if err != nil {
			c.log("memory context for agent %s: %v; registering again", agent, err)
			known = false
		}
	}
	if !known {
		result, err = c.Identity.Guide.Guide(ctx, c.Identity.RegistrationKey, "", "")
		if err != nil {
			c.log("memory agent registration: %v", err)
			return
		}
		agent = result.AgentID
		if err := c.Identity.Agents.Remember(ctx, c.Identity.Store, agent, c.Identity.RegistrationKey); err != nil {
			c.log("memory agent registry: %v", err)
		}
		c.log("memory agent %s registered in %s", agent, c.Identity.Store)
		// The registration's context is the agent's, not this session's.
		result, err = c.Identity.Guide.Guide(ctx, "", agent, contextKey(id))
		if err != nil {
			c.log("memory context for session %s: %v", id, err)
			return
		}
	}
	instance.KMPAgent, instance.KMPContext = agent, result.ContextID
	if err := s.SetInstance(instance); err != nil {
		c.log("session instance: %v", err)
		return
	}
	c.log("memory context %s opened for session %s", result.ContextID, id)
}

// withSessionContext puts the session's memory context in the argument
// the tool's surface names: the console holds the session's context, and
// a context the model names is replaced (a model's call runs in the
// session's context, never in one of its own; decision G6).
func withSessionContext(s domain.Session, tool domain.AvailableTool, arguments root.JSONValue) root.JSONValue {
	argument := tool.Surface.ContextArgument
	if tool.Identity.Kind != domain.ToolKindPlugin || argument == "" {
		return arguments
	}
	instance, ok := s.Instance()
	if !ok || instance.KMPContext == "" {
		return arguments
	}
	var call map[string]json.RawMessage
	if json.Unmarshal(arguments.Bytes(), &call) != nil || call == nil {
		return arguments
	}
	call[argument] = json.RawMessage(fmt.Sprintf("%q", instance.KMPContext))
	data, err := json.Marshal(call)
	if err != nil {
		return arguments
	}
	value, err := root.NewJSONObject(data)
	if err != nil {
		return arguments
	}
	return value
}
