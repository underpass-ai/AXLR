package application

import root "github.com/underpass-ai/AXLR/domain"

// ToolArgumentValidationPort validates the real frozen tool schema before
// approval can lead to an effect. It must not rewrite the supplied arguments.
type ToolArgumentValidationPort interface {
	Validate(root.ToolDefinition, root.JSONValue) error
}
