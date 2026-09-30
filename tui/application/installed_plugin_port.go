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
	Guidance(context.Context) (string, error)
}
