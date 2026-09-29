package domain

import (
	"errors"
	"regexp"
)

var pluginIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

type PluginID string

func NewPluginID(raw string) (PluginID, error) {
	if !pluginIDPattern.MatchString(raw) {
		return "", errors.New("invalid plugin ID")
	}
	return PluginID(raw), nil
}

func (id PluginID) String() string { return string(id) }
