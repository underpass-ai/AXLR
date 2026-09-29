package openrouter

import "github.com/underpass-ai/AXLR/domain"

func mapRequest(req domain.CompletionRequest) (requestDTO, error) {
	if err := req.Validate(); err != nil {
		return requestDTO{}, err
	}
	wire := requestDTO{Model: string(req.Model), Messages: make([]messageDTO, 0, len(req.Messages)), Stream: false}
	for _, message := range req.Messages {
		mapped := messageDTO{Role: string(message.Role), ToolCallID: string(message.ToolCallID)}
		if message.Role != domain.RoleAssistant || message.Content != "" {
			content := string(message.Content)
			mapped.Content = &content
		}
		for _, call := range message.ToolCalls {
			mappedCall := toolCallDTO{ID: string(call.ID), Type: "function"}
			mappedCall.Function.Name = string(call.Name)
			mappedCall.Function.Arguments = string(call.Arguments.Bytes())
			mapped.ToolCalls = append(mapped.ToolCalls, mappedCall)
		}
		wire.Messages = append(wire.Messages, mapped)
	}
	for _, definition := range req.Tools {
		mapped := toolDTO{Type: "function"}
		mapped.Function.Name = string(definition.Name)
		mapped.Function.Description = string(definition.Description)
		mapped.Function.Parameters = definition.Parameters.Bytes()
		wire.Tools = append(wire.Tools, mapped)
	}
	return wire, nil
}
