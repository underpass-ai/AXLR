package dto

// ToolSurface is domain.ToolSurface as the session file keeps it.
type ToolSurface struct {
	Role   string `json:"role,omitempty"`
	Hidden string `json:"hidden,omitempty"`
	// ContextArgument is read from sessions written before KMP 1.0, which
	// named the argument that carried the session's memory context; the
	// session file refuses unknown fields, so it stays, and is ignored.
	ContextArgument string `json:"context_argument,omitempty"`
	MemoryWrite     bool   `json:"memory_write,omitempty"`
}
