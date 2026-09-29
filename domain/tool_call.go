package domain

import "errors"

type ToolCall struct {
	ID        ToolCallID
	Name      ToolName
	Arguments JSONValue
}

func (c ToolCall) validate() error {
	if _, err := NewToolCallID(string(c.ID)); err != nil {
		return err
	}
	if _, err := NewToolName(string(c.Name)); err != nil {
		return err
	}
	if !c.Arguments.isObject() {
		return errors.New("tool call arguments must be a JSON object")
	}
	return nil
}
