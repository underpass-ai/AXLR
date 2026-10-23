package domain

import (
	"bytes"
	"errors"
)

type Message struct {
	Role       MessageRole
	Content    Text
	ToolCalls  []ToolCall
	ToolCallID ToolCallID
	// Reasoning is the provider's record of the reasoning behind an
	// assistant message (OpenRouter's reasoning_details array: text, a
	// summary, encrypted data, signatures), kept unmodified and sent back
	// with the message. A model that called tools then continues the
	// reasoning that led to them: until 23 October 2026 AXLR dropped it, and
	// claude-haiku-5.5 thought again from nothing before every write, 15,000
	// to 30,000 reasoning tokens each. Empty when the provider sent none.
	Reasoning JSONValue
}

// HasReasoning reports whether the message carries the provider's reasoning.
func (m Message) HasReasoning() bool { return len(m.Reasoning.raw) > 0 }

func (m Message) Validate() error {
	if _, err := NewText(string(m.Content)); err != nil {
		return err
	}
	if m.HasReasoning() {
		if m.Role != RoleAssistant {
			return errors.New("only an assistant message carries reasoning")
		}
		if trimmed := bytes.TrimSpace(m.Reasoning.raw); len(trimmed) == 0 || trimmed[0] != '[' {
			return errors.New("message reasoning must be a JSON array")
		}
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
