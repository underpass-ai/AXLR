package application

import "encoding/json"

// ForgedTool is one registered tool of a workspace: what the model sees, the
// command the console runs from the workspace root with the arguments as JSON
// on stdin, and the SHA-256 of each file it was forged with.
type ForgedTool struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	InputSchema json.RawMessage   `json:"input_schema"`
	Program     string            `json:"program"`
	Args        []string          `json:"args"`
	Files       map[string]string `json:"files"`
	ForgedAt    string            `json:"forged_at,omitempty"`
	Session     string            `json:"session,omitempty"`
	// Broken is why Verify refuses the tool now (its files changed or are
	// missing), set for axlr_tools only; it is never stored.
	Broken string `json:"-"`
}
