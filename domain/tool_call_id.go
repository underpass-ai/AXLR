package domain

import "errors"

type ToolCallID string

func NewToolCallID(value string) (ToolCallID, error) {
	if !validModelIdentifier(value) {
		return "", errors.New("invalid tool call ID")
	}
	return ToolCallID(value), nil
}
