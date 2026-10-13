package dto

// ToolSurface is domain.ToolSurface as the session file keeps it.
type ToolSurface struct {
	Role            string `json:"role,omitempty"`
	Hidden          string `json:"hidden,omitempty"`
	ContextArgument string `json:"context_argument,omitempty"`
	MemoryWrite     bool   `json:"memory_write,omitempty"`
}
