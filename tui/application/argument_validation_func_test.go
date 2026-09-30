package application

import root "github.com/underpass-ai/AXLR/domain"

type argumentValidationFunc func(root.ToolDefinition, root.JSONValue) error

func (f argumentValidationFunc) Validate(tool root.ToolDefinition, args root.JSONValue) error {
	return f(tool, args)
}
