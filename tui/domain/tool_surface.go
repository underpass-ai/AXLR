package domain

import "errors"

// PluginRole is the part an engine plays for the console: its memory or
// its orchestrator. The console runs both protocols itself (the session's
// context, agent, instance and turns), so it must know which plugin is
// which without knowing the engine's name; the adapter that registers
// the plugin says.
type PluginRole string

const (
	// PluginRoleNone is a plugin the console only bridges.
	PluginRoleNone PluginRole = ""
	// PluginRoleMemory is the engine that keeps the durable memory.
	PluginRoleMemory PluginRole = "memory"
	// PluginRoleOrchestrator is the engine that keeps the ceremony state.
	PluginRoleOrchestrator PluginRole = "orchestrator"
)

// ToolSurface is what the console owns of one plugin tool, decided by the
// engine adapter when the tool is registered and frozen in the session's
// snapshot with the tool: whether the model may call it, which argument
// carries the session's memory context and whether a call of it is a
// memory write.
type ToolSurface struct {
	Role PluginRole
	// Hidden is the refusal the bridge answers and axlr_tools shows when
	// the console keeps the tool to itself; empty when the model may call
	// it.
	Hidden string
	// ContextArgument names the argument the console fills with the
	// session's memory context on every call; empty when the tool takes
	// none.
	ContextArgument string
	// MemoryWrite is true for the engine's memory write: the console's
	// reminder, the turn record and the model's orientation count it.
	MemoryWrite bool
}

// Validate refuses a surface that names a role the console does not play.
func (s ToolSurface) Validate() error {
	switch s.Role {
	case PluginRoleNone, PluginRoleMemory, PluginRoleOrchestrator:
		return nil
	}
	return errors.New("tool surface role must be memory, orchestrator or empty")
}
