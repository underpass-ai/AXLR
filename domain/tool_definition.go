package domain

import "errors"

type ToolDefinition struct {
	Name        ToolName
	Description Text
	Parameters  JSONValue
}

func (d ToolDefinition) validate() error {
	if _, err := NewToolName(string(d.Name)); err != nil {
		return err
	}
	if _, err := NewText(string(d.Description)); err != nil {
		return err
	}
	if !d.Parameters.isObject() {
		return errors.New("tool parameters must be a JSON object")
	}
	return nil
}
