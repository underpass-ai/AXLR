package application

import (
	"context"
	"errors"
)

// ErrEngineUpdatePlatform reports a host with no official MADE/KMP package.
var ErrEngineUpdatePlatform = errors.New("no official MADE or KMP package for this platform")

// EngineUpdatePort updates the explicitly configured local MADE/KMP packages.
// Running MCP processes keep their current executable until AXLR restarts.
type EngineUpdatePort interface {
	Update(context.Context) ([]EngineUpdateResult, error)
}

type EngineUpdateResult struct {
	Engine          string
	PreviousVersion string
	Version         string
	Status          string // installed, updated, current, skipped, failed
	RestartRequired bool
	Error           string
	// Detail says what an installation prepared, or what is left to do.
	Detail string
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

// EngineRegistration is the mcp.json entry for an engine package that AXLR
// installed itself: its launcher, profile and explicit child environment.
type EngineRegistration struct {
	Engine      string
	Launcher    string
	Name        string
	Description string
	Purpose     string
	Approval    string
	Env         map[string]string
	EnvFrom     map[string]string
}

// EngineInstallPort prepares and connects an engine package mcp.json lacks;
// packageRoot is the verified, extracted release. The detail is shown to the
// person beside the installed version.
type EngineInstallPort interface {
	InstallEngine(ctx context.Context, engine, packageRoot string) (detail string, err error)
}

// EngineVersionPort reads the version of each engine installed for this
// console, by engine ("kmp", "made"); an engine it cannot run is absent.
type EngineVersionPort interface {
	InstalledVersions(context.Context) (map[string]string, error)
}
