package application

import (
	"context"
	"encoding/json"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The console's KMP identity (decision D3 of the lifecycle integration
// design): one agent per store, registered with the install's key, and one
// context per session, opened at its first prompt and passed by the
// console on every KMP call the session makes. The model never registers,
// never sees an id and never opens a context.

// KMPAgentsPort keeps the agent the console registered in each store.
type KMPAgentsPort interface {
	Agent(ctx context.Context, store string) (string, bool)
	Remember(ctx context.Context, store, agentID, registrationKey string) error
}

// KMPIdentity is this launch's KMP identity: the store it opened, the
// install's registration key and where registrations are kept.
type KMPIdentity struct {
	// Store is the resolved store path, the key of the agent registry.
	Store string
	// RegistrationKey is axlr:<install id>; the same key names the same
	// agent in a store.
	RegistrationKey string
	Agents          KMPAgentsPort
	Guide           MemoryIdentityPort
	// Recall wakes memory for the recall note; nil appends none.
	Recall RecallPort
	// UserAbout is the person's about, user:<login name>, woken with the
	// project's (decision F6); "" when the login name is unknown.
	UserAbout string
}

func (i *KMPIdentity) usable() bool {
	return i != nil && i.Store != "" && i.RegistrationKey != "" && i.Agents != nil && i.Guide != nil
}

// contextKey names the session's context; the same key reopens the same
// context when the sidecar was lost.
func contextKey(id domain.SessionID) string { return "axlr:session:" + string(id) }

// openContext opens the session's KMP context when it has none: the store's
// agent is registered first when the registry holds none, and registered
// again when KMP no longer knows the recorded one (the store was emptied or
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
			c.log("KMP context for agent %s: %v; registering again", agent, err)
			known = false
		}
	}
	if !known {
		result, err = c.Identity.Guide.Guide(ctx, c.Identity.RegistrationKey, "", "")
		if err != nil {
			c.log("KMP agent registration: %v", err)
			return
		}
		agent = result.AgentID
		if err := c.Identity.Agents.Remember(ctx, c.Identity.Store, agent, c.Identity.RegistrationKey); err != nil {
			c.log("KMP agent registry: %v", err)
		}
		c.log("KMP agent %s registered in %s", agent, c.Identity.Store)
		// The registration's context is the agent's, not this session's.
		result, err = c.Identity.Guide.Guide(ctx, "", agent, contextKey(id))
		if err != nil {
			c.log("KMP context for session %s: %v", id, err)
			return
		}
	}
	instance.KMPAgent, instance.KMPContext = agent, result.ContextID
	if err := s.SetInstance(instance); err != nil {
		c.log("session instance: %v", err)
		return
	}
	c.log("KMP context %s opened for session %s", result.ContextID, id)
}

// withSessionContext puts the session's KMP context in a kmp call's
// arguments: every KMP verb but kmp_guide takes context_id, the console
// holds the session's context, and a context the model names is replaced
// (a model's call runs in the session's context, never in one of its own;
// decision G6).
func withSessionContext(s domain.Session, tool domain.AvailableTool, arguments root.JSONValue) root.JSONValue {
	if tool.Identity.Kind != domain.ToolKindPlugin || tool.Identity.Plugin.PluginID != "kmp" || tool.Identity.Plugin.ToolName == "kmp_guide" {
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
	call["context_id"] = json.RawMessage(fmt.Sprintf("%q", instance.KMPContext))
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
