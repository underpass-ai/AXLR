package domain

import axlr "github.com/underpass-ai/AXLR/domain"

type AvailableTool struct {
	Definition axlr.ToolDefinition
	Identity   ToolIdentity
	// Surface is what the console owns of a plugin tool; zero for the
	// console's own tools and for a plugin it only bridges.
	Surface ToolSurface
}

func validateTools(model axlr.ModelID, tools []AvailableTool) error {
	definitions := make([]axlr.ToolDefinition, len(tools))
	for i, tool := range tools {
		if err := tool.Surface.Validate(); err != nil {
			return err
		}
		if err := tool.Identity.Validate(); err != nil {
			return err
		}
		definitions[i] = tool.Definition
	}
	return (axlr.CompletionRequest{Model: model, Messages: []axlr.Message{{Role: axlr.RoleUser, Content: "validate"}}, Tools: definitions}).Validate()
}
