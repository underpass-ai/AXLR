package domain

import (
	"errors"
	"regexp"
)

var pluginToolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type PluginToolName string

func NewPluginToolName(raw string) (PluginToolName, error) {
	if len(raw) == 0 || len(raw) > 128 || !pluginToolNamePattern.MatchString(raw) {
		return "", errors.New("invalid plugin tool name")
	}
	return PluginToolName(raw), nil
}

func (name PluginToolName) String() string { return string(name) }
