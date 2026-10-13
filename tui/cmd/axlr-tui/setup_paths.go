package main

import (
	"fmt"
	"path/filepath"
)

type setupPaths struct {
	settings, credentials, mcp, data string
}

// resolveSetupPaths finds settings.json, credentials.json, mcp.json and the
// data directory exactly as the console does.
func resolveSetupPaths(getenv func(string) string, mcpConfig string) (setupPaths, error) {
	home := getenv("HOME")
	base := func(variable string, fallback ...string) (string, error) {
		if dir := getenv(variable); filepath.IsAbs(dir) {
			return dir, nil
		}
		if !filepath.IsAbs(home) {
			return "", fmt.Errorf("absolute HOME or %s is required", variable)
		}
		return filepath.Join(append([]string{home}, fallback...)...), nil
	}
	config, err := base("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return setupPaths{}, err
	}
	data, err := base("XDG_DATA_HOME", ".local", "share")
	if err != nil {
		return setupPaths{}, err
	}
	if mcpConfig == "" {
		mcpConfig = filepath.Join(config, "axlr", "mcp.json")
	}
	return setupPaths{
		settings:    filepath.Join(config, "axlr", "settings.json"),
		credentials: filepath.Join(config, "axlr", "credentials.json"),
		mcp:         mcpConfig,
		data:        data,
	}, nil
}
