package mcpclient

import (
	"errors"
	"regexp"
)

var serverNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// ServerName is a stable local identity for one configured MCP server.
type ServerName string

func NewServerName(raw string) (ServerName, error) {
	if !serverNamePattern.MatchString(raw) {
		return "", errors.New("invalid MCP server name")
	}
	return ServerName(raw), nil
}

func (n ServerName) String() string { return string(n) }
