package openrouter

import (
	"errors"
	"github.com/underpass-ai/AXLR/domain"
)

func mapRequest(req domain.CompletionRequest) (requestDTO, error) {
	if err := req.Validate(); err != nil {
		return requestDTO{}, err
	}
	// History may contain denied unknown calls or tools removed between turns.
	// Tool result correlation is validated by CompletionRequest; historical
	// definitions are not required for sending their results to the provider.
	// OpenRouter still requires the current tool surface on follow-up requests.
	for _, message := range req.Messages {
		if message.Role == domain.RoleTool && len(req.Tools) == 0 {
			return requestDTO{}, errors.New("OpenRouter tool result requires current tool definitions")
		}
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
