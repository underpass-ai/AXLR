package application

import "context"

// InstalledPlugin is a Codex plugin package, distinct from an AXLR MCP server.
type InstalledPlugin struct {
	ID, Name, Marketplace, Version, Source, InstallPolicy, AuthPolicy string
	Description                                                       string
	Components                                                        []string
	Installed, Enabled                                                bool
}

type InstalledPluginPort interface {
	List(context.Context, bool) ([]InstalledPlugin, error)
	Install(context.Context, string) error
	AddMarketplace(context.Context, string) error
}
