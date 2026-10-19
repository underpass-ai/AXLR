package storage

import (
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// MCPConfiguration joins process registrations with their safe capability metadata.
type MCPConfiguration struct {
	Registrations []plugins.Registration
	Profiles      []domain.PluginProfile
	// Unreadable says, one line each, which entries were left out because
	// their manifest could not be loaded; the console starts without them.
	Unreadable []string
}
