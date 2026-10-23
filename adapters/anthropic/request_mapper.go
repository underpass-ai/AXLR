package anthropic

import (
	"encoding/json"
	"errors"

	"github.com/underpass-ai/AXLR/domain"
)

// DefaultMaxTokens caps a reply when the configuration gives no max_tokens:
// room for a large write, within what every current Claude model allows.
const DefaultMaxTokens = 32000

// mapRequest turns the domain request into the Messages API shape. System
// messages at the start become the top-level system blocks, with a cache
// breakpoint on the last one so the fixed prefix stays readable after a cut
// of the history; a system message later in the conversation becomes user
// text, since the API alternates user and assistant. Consecutive messages
// of one role merge into one message: every tool result of a turn goes back
// in a single user message, as the API requires. An assistant message
// carries its thinking blocks first, unmodified, then its text, then its
// tool_use blocks.
func mapRequest(req domain.CompletionRequest, maxTokens int, thinking bool) (requestDTO, error) {
	if err := req.Validate(); err != nil {
		return requestDTO{}, err
	}
	for _, message := range req.Messages {
		if message.Role == domain.RoleTool && len(req.Tools) == 0 {
			return requestDTO{}, errors.New("Anthropic tool result requires current tool definitions")
		}
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	wire := requestDTO{Model: string(req.Model), MaxTokens: maxTokens, CacheControl: &cacheControlDTO{Type: "ephemeral"}}
	if thinking {
		wire.Thinking = thinkingConfig
	}
	var current *messageDTO
	add := func(role string, blocks ...any) {
		if current == nil || current.Role != role {
			wire.Messages = append(wire.Messages, messageDTO{Role: role})
			current = &wire.Messages[len(wire.Messages)-1]
		}
		current.Content = append(current.Content, blocks...)
	}
	for _, message := range req.Messages {
		switch message.Role {
		case domain.RoleSystem:
			if len(wire.Messages) == 0 {
				wire.System = append(wire.System, blockDTO{Type: "text", Text: string(message.Content)})
				continue
			}
			add("user", blockDTO{Type: "text", Text: string(message.Content)})
		case domain.RoleUser:
			add("user", blockDTO{Type: "text", Text: string(message.Content)})
		case domain.RoleTool:
			add("user", blockDTO{Type: "tool_result", ToolUseID: string(message.ToolCallID), Content: string(message.Content)})
		case domain.RoleAssistant:
			var blocks []any
			if message.HasReasoning() {
				var raw []json.RawMessage
				if err := json.Unmarshal(message.Reasoning.Bytes(), &raw); err != nil {
					return requestDTO{}, errors.New("malformed Anthropic thinking blocks")
				}
				for _, block := range raw {
					blocks = append(blocks, block)
				}
			}
			if message.Content != "" {
				blocks = append(blocks, blockDTO{Type: "text", Text: string(message.Content)})
			}
			for _, call := range message.ToolCalls {
				blocks = append(blocks, blockDTO{Type: "tool_use", ID: string(call.ID), Name: string(call.Name), Input: call.Arguments.Bytes()})
			}
			add("assistant", blocks...)
		}
	}
	if len(wire.System) > 0 {
		wire.System[len(wire.System)-1].CacheControl = &cacheControlDTO{Type: "ephemeral"}
	}
	for _, definition := range req.Tools {
		wire.Tools = append(wire.Tools, toolDTO{Name: string(definition.Name), Description: string(definition.Description), InputSchema: inputSchema(definition.Parameters.Bytes())})
	}
	return wire, nil
}

// inputSchema is a tool's parameters as the API reads them: an object
// schema that says so. AXLR's own tools declare "type": "object"; an MCP
// server's schema may leave it out, which the chat completions shape
// tolerates and the Messages API refuses.
func inputSchema(parameters []byte) json.RawMessage {
	var schema map[string]json.RawMessage
	if json.Unmarshal(parameters, &schema) != nil || schema == nil {
		return parameters
	}
	if _, typed := schema["type"]; typed {
		return parameters
	}
	schema["type"] = json.RawMessage(`"object"`)
	encoded, err := json.Marshal(schema)
	if err != nil {
		return parameters
	}
	return encoded
}
