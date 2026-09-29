package domain

import "errors"

type CompletionRequest struct {
	Model    ModelID
	Messages []Message
	Tools    []ToolDefinition
}

func (r CompletionRequest) Validate() error {
	if _, err := NewModelID(string(r.Model)); err != nil {
		return err
	}
	if len(r.Messages) == 0 {
		return errors.New("completion requires messages")
	}
	tools := make(map[ToolName]bool, len(r.Tools))
	for _, definition := range r.Tools {
		if err := definition.validate(); err != nil {
			return err
		}
		if tools[definition.Name] {
			return errors.New("duplicate tool name")
		}
		tools[definition.Name] = true
	}
	calls := make(map[ToolCallID]bool)
	results := make(map[ToolCallID]bool)
	for _, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return err
		}
		if message.Role == RoleAssistant {
			for _, call := range message.ToolCalls {
				if calls[call.ID] {
					return errors.New("duplicate tool call ID")
				}
				calls[call.ID] = true
			}
		}
		if message.Role == RoleTool {
			if !calls[message.ToolCallID] {
				return errors.New("tool result has no preceding call")
			}
			if results[message.ToolCallID] {
				return errors.New("duplicate tool result")
			}
			results[message.ToolCallID] = true
		}
	}
	return nil
}
