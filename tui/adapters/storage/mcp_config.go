package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

const maxMCPConfigBytes = 64 << 10

var mcpEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// LoadMCPConfig reads explicit, persistent plugin registrations. A missing
// configuration means no external servers were selected for AXLR.
func LoadMCPConfig(path string, getenv func(string) string) ([]plugins.Registration, error) {
	configuration, err := LoadMCPConfiguration(path, getenv)
	return configuration.Registrations, err
}

// LoadMCPConfiguration validates registrations and their explicit plugin policies.
func LoadMCPConfiguration(path string, getenv func(string) string) (MCPConfiguration, error) {
	if getenv == nil {
		return MCPConfiguration{}, errors.New("MCP environment resolver is required")
	}
	config, err := readMCPConfig(path)
	if err != nil {
		return MCPConfiguration{}, err
	}
	profiles := make([]domain.PluginProfile, 0, len(config.Plugins))
	seen := map[root.PluginID]bool{}
	registrations := make([]plugins.Registration, 0, len(config.Plugins))
	for _, selected := range config.Plugins {
		manifest, err := plugins.LoadManifest(selected.Manifest)
		if err != nil {
			return MCPConfiguration{}, err
		}
		if seen[manifest.ID] {
			return MCPConfiguration{}, errors.New("duplicate MCP plugin ID")
		}
		seen[manifest.ID] = true
		profile := domain.PluginProfile{ID: manifest.ID, Name: root.Text(selected.Name), Description: root.Text(selected.Description), Purpose: domain.PluginPurpose(selected.Purpose), Approval: domain.ApprovalMode(selected.Approval)}
		if profile.Name == "" {
			profile.Name = root.Text(manifest.ID.String())
		}
		if profile.Purpose == "" {
			profile.Purpose = domain.PluginPurposeTools
		}
		if profile.Approval == "" {
			profile.Approval = domain.ApprovalManual
		}
		if err := profile.Validate(); err != nil {
			return MCPConfiguration{}, err
		}
		profiles = append(profiles, profile)
		if len(selected.Env)+len(selected.EnvFrom) > 64 {
			return MCPConfiguration{}, errors.New("too many MCP plugin environment entries")
		}
		values := make(map[string]string, len(selected.Env)+len(selected.EnvFrom))
		for key, value := range selected.Env {
			if !mcpEnvironmentName.MatchString(key) || key == "OPENROUTER_API_KEY" || strings.ContainsRune(value, 0) {
				return MCPConfiguration{}, errors.New("invalid MCP plugin environment")
			}
			values[key] = value
		}
		for key, source := range selected.EnvFrom {
			if !mcpEnvironmentName.MatchString(key) || !mcpEnvironmentName.MatchString(source) || source == "OPENROUTER_API_KEY" || key == "OPENROUTER_API_KEY" {
				return MCPConfiguration{}, errors.New("invalid MCP plugin environment source")
			}
			if _, exists := values[key]; exists {
				return MCPConfiguration{}, errors.New("duplicate MCP plugin environment key")
			}
			values[key] = getenv(source)
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var env []string
		for _, key := range keys {
			env = append(env, key+"="+values[key])
		}
		registration, err := plugins.NewRegistration(manifest, env)
		if err != nil {
			return MCPConfiguration{}, err
		}
		registrations = append(registrations, registration)
	}
	return MCPConfiguration{Registrations: registrations, Profiles: profiles}, nil
}

// checkMCPConfigFile names the path, the first failing requirement and the
// fix: a regular file, private permissions (no group or other access) and a
// size within maxMCPConfigBytes. On 10 October 2026 a mcp.json copied under
// umask 002 (mode 0664) stopped the console with one sentence that named
// neither the file nor which of the three requirements it failed.
func checkMCPConfigFile(path string, info os.FileInfo) error {
	switch {
	case !info.Mode().IsRegular():
		return fmt.Errorf("MCP config %s is not a regular file; replace it with a regular file of mode 0600", path)
	case !privateRegular(info):
		return fmt.Errorf("MCP config %s has mode %04o and must not be readable or writable by other accounts; run chmod 600 %s", path, info.Mode().Perm(), path)
	case info.Size() > maxMCPConfigBytes:
		return fmt.Errorf("MCP config %s is %d bytes; the limit is %d bytes (64 KiB)", path, info.Size(), maxMCPConfigBytes)
	}
	return nil
}

// privateLockError names a lock file that failed privateRegular, its mode
// and the fix, as checkMCPConfigFile does for the configuration itself.
func privateLockError(kind, path string, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s %s is not a regular file; remove it and start the console again", kind, path)
	}
	return fmt.Errorf("%s %s has mode %04o and must not be readable or writable by other accounts; run chmod 600 %s", kind, path, info.Mode().Perm(), path)
}

func readMCPConfig(path string) (dto.MCPConfig, error) {
	if !filepath.IsAbs(path) {
		return dto.MCPConfig{}, errors.New("MCP config path must be absolute")
	}
	// Open and verify the same descriptor: a path swap cannot bypass privacy checks.
	file, err := openNoFollow(path, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return dto.MCPConfig{}, nil
	}
	if err != nil {
		return dto.MCPConfig{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return dto.MCPConfig{}, err
	}
	if err := checkMCPConfigFile(path, info); err != nil {
		return dto.MCPConfig{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxMCPConfigBytes+1))
	if err != nil {
		return dto.MCPConfig{}, err
	}
	if len(data) > maxMCPConfigBytes || !utf8.Valid(data) {
		return dto.MCPConfig{}, errors.New("invalid MCP config size or UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config dto.MCPConfig
	if err := decoder.Decode(&config); err != nil {
		return dto.MCPConfig{}, errors.New("invalid MCP config JSON")
	}
	if decoder.Decode(new(any)) != io.EOF || config.Version != 1 || len(config.Plugins) > 32 {
		return dto.MCPConfig{}, errors.New("invalid MCP config version or plugin count")
	}
	return config, nil
}
