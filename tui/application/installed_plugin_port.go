package application

import "context"

// InstalledPlugin describes one package owned by AXLR, distinct from a server connection.
type InstalledPlugin struct {
	ID, Name, Version, Source, Description string
	Components                             []string
	Installed, Builtin                     bool
}

type InstalledPluginPort interface {
	List(context.Context, bool) ([]InstalledPlugin, error)
	Install(context.Context, string) error
	AddSource(context.Context, string) error
	// Update replaces an installed package with a fresh copy of its source.
	Update(context.Context, string) error
	// Uninstall removes an installed package and its MCP registrations.
	Uninstall(context.Context, string) error
	Guidance(context.Context) (string, error)
}
