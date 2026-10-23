package openai

import (
	"encoding/json"
	"errors"

	"github.com/underpass-ai/AXLR/domain"
)

// mapRequest turns the domain request into the Responses API shape: every
// message an input item, a tool result a function_call_output, and an
// assistant turn its reasoning items first, replayed as they came, then its
// text, then its function calls. The request keeps no server state.
func mapRequest(req domain.CompletionRequest, reasoning bool) (requestDTO, error) {
	if err := req.Validate(); err != nil {
		return requestDTO{}, err
	}
	wire := requestDTO{Model: string(req.Model), Input: make([]any, 0, len(req.Messages))}
	if reasoning {
		wire.Reasoning = reasoningConfig
		wire.Include = encryptedReasoning
	}
	for _, message := range req.Messages {
		switch message.Role {
		case domain.RoleSystem, domain.RoleUser:
			wire.Input = append(wire.Input, itemDTO{Role: string(message.Role), Content: string(message.Content)})
		case domain.RoleTool:
			wire.Input = append(wire.Input, itemDTO{Type: "function_call_output", CallID: string(message.ToolCallID), Output: string(message.Content)})
		case domain.RoleAssistant:
			if message.HasReasoning() {
				var items []json.RawMessage
				if err := json.Unmarshal(message.Reasoning.Bytes(), &items); err != nil {
					return requestDTO{}, errors.New("malformed OpenAI reasoning items")
				}
				for _, item := range items {
					wire.Input = append(wire.Input, item)
				}
			}
			if message.Content != "" {
				wire.Input = append(wire.Input, itemDTO{Role: "assistant", Content: string(message.Content)})
			}
			for _, call := range message.ToolCalls {
				wire.Input = append(wire.Input, itemDTO{Type: "function_call", CallID: string(call.ID), Name: string(call.Name), Arguments: string(call.Arguments.Bytes())})
			}
		}
	}
	for _, definition := range req.Tools {
		wire.Tools = append(wire.Tools, toolDTO{Type: "function", Name: string(definition.Name), Description: string(definition.Description), Parameters: definition.Parameters.Bytes()})
	}
	return wire, nil
}
