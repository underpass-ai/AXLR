package mcpclient

import (
	"errors"
	"regexp"
)

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ToolName is the name advertised by an MCP server.
type ToolName string

func NewToolName(raw string) (ToolName, error) {
	if len(raw) == 0 || len(raw) > 128 || !toolNamePattern.MatchString(raw) {
		return "", errors.New("invalid MCP tool name")
	}
	return ToolName(raw), nil
}

func (n ToolName) String() string { return string(n) }
