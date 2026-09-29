package domain

import axlr "github.com/underpass-ai/AXLR/domain"

type AvailableTool struct {
	Definition axlr.ToolDefinition
	Identity   ToolIdentity
}

func validateTools(model axlr.ModelID, tools []AvailableTool) error {
	definitions := make([]axlr.ToolDefinition, len(tools))
	for i, tool := range tools {
		if err := tool.Identity.Validate(); err != nil {
			return err
		}
		definitions[i] = tool.Definition
	}
	return (axlr.CompletionRequest{Model: model, Messages: []axlr.Message{{Role: axlr.RoleUser, Content: "validate"}}, Tools: definitions}).Validate()
}
