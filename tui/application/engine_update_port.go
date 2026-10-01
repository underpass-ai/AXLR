package application

import "context"

// EngineUpdatePort updates the explicitly configured local MADE/KMP packages.
// Running MCP processes keep their current executable until AXLR restarts.
type EngineUpdatePort interface {
	Update(context.Context) ([]EngineUpdateResult, error)
}

type EngineUpdateResult struct {
	Engine          string
	PreviousVersion string
	Version         string
	Status          string // updated, current, skipped, failed
	RestartRequired bool
	Error           string
}

type EngineUpdateTarget struct {
	Engine   string
	Manifest string
	Command  string
}

type EngineConfigurationPort interface {
	EngineTargets(context.Context) ([]EngineUpdateTarget, error)
	ActivateEngine(context.Context, EngineUpdateTarget, string) error
}
