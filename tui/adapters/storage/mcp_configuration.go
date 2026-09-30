package storage

import (
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// MCPConfiguration joins process registrations with their safe capability metadata.
type MCPConfiguration struct {
	Registrations []plugins.Registration
	Profiles      []domain.PluginProfile
}
