package domain

import "errors"

type Message struct {
	Role       MessageRole
	Content    Text
	ToolCalls  []ToolCall
	ToolCallID ToolCallID
}

func (m Message) Validate() error {
	if _, err := NewText(string(m.Content)); err != nil {
		return err
	}
	switch m.Role {
	case RoleSystem, RoleUser:
		if m.Content == "" || len(m.ToolCalls) != 0 || m.ToolCallID != "" {
			return errors.New("invalid system or user message")
		}
	case RoleAssistant:
		if m.ToolCallID != "" || (m.Content == "" && len(m.ToolCalls) == 0) {
			return errors.New("invalid assistant message")
		}
		for _, call := range m.ToolCalls {
			if err := call.validate(); err != nil {
				return err
			}
		}
	case RoleTool:
		if _, err := NewToolCallID(string(m.ToolCallID)); err != nil {
			return err
		}
		if len(m.ToolCalls) != 0 {
			return errors.New("tool result cannot request tools")
		}
	default:
		return errors.New("invalid message role")
	}
	return nil
}
