package domain

import "errors"

type ToolName string

func NewToolName(value string) (ToolName, error) {
	if !validModelIdentifier(value) {
		return "", errors.New("invalid tool name")
	}
	return ToolName(value), nil
}
