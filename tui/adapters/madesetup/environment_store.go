package madesetup

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
)

// EnvironmentStore persists the work identity on the MADE entry of mcp.json.
type EnvironmentStore interface {
	SetPluginEnvironment(ctx context.Context, id root.PluginID, key, value string) error
}
