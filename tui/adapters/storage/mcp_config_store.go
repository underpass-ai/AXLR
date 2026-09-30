package storage

import (
	"context"
	"encoding/json"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// MCPConfigStore persists explicit authorization without exposing process environments.
type MCPConfigStore struct {
	Path          string
	mu            sync.Mutex
	syncDirectory func(*os.File) error
}

func (s *MCPConfigStore) SaveApproval(ctx context.Context, id root.PluginID, mode domain.ApprovalMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := root.NewPluginID(id.String()); err != nil {
		return err
	}
	if err := mode.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := acquireMCPConfigLock(ctx, s.Path)
	if err != nil {
		return err
	}
	defer release()
	config, err := readMCPConfig(s.Path)
	if err != nil {
		return err
	}
	found := false
	for i, selected := range config.Plugins {
		manifest, err := plugins.LoadManifest(selected.Manifest)
		if err != nil {
			return errors.New("cannot validate configured plugin")
		}
		if manifest.ID == id {
			if found {
				return errors.New("duplicate MCP plugin ID")
			}
			config.Plugins[i].Approval = string(mode)
			found = true
		}
	}
	if !found {
		return errors.New("plugin is not in the persistent MCP configuration")
	}
	return s.write(ctx, config)
}

// AddManifest installs a third-party stdio MCP server from an AXLR manifest.
// Its initial policy is manual; existing KMP/MADE entries are preserved.
func (s *MCPConfigStore) AddManifest(ctx context.Context, path string) error {
	return s.AddManifestWithEnvironment(ctx, path, nil, nil)
}

// AddManifestWithEnvironment persists explicit values and host variable names for a package MCP.
func (s *MCPConfigStore) AddManifestWithEnvironment(ctx context.Context, path string, envFrom, env map[string]string) error {
	for key, source := range envFrom {
		if !mcpEnvironmentName.MatchString(key) || !mcpEnvironmentName.MatchString(source) || key == "OPENROUTER_API_KEY" || source == "OPENROUTER_API_KEY" {
			return errors.New("invalid MCP environment source")
		}
	}
	for key, value := range env {
		if !mcpEnvironmentName.MatchString(key) || key == "OPENROUTER_API_KEY" || strings.ContainsRune(value, 0) {
			return errors.New("invalid MCP environment value")
		}
		if _, exists := envFrom[key]; exists {
			return errors.New("duplicate MCP environment key")
		}
	}
	if len(envFrom)+len(env) > 64 {
		return errors.New("too many MCP environment entries")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	manifest, err := plugins.LoadManifest(path)
	if err != nil {
		return err
	}
	if _, err := plugins.NewRegistration(manifest, nil); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := acquireMCPConfigLock(ctx, s.Path)
	if err != nil {
		return err
	}
	defer release()
	config, err := readMCPConfig(s.Path)
	if err != nil {
		return err
	}
	for _, selected := range config.Plugins {
		existing, err := plugins.LoadManifest(selected.Manifest)
		if err != nil {
			return errors.New("cannot validate configured MCP server")
		}
		if existing.ID == manifest.ID {
			return errors.New("MCP server is already installed")
		}
	}
	if len(config.Plugins) >= 32 {
		return errors.New("MCP server limit reached")
	}
	config.Version = 1
	config.Plugins = append(config.Plugins, dto.MCPPluginConfig{Manifest: path, Name: manifest.ID.String(), Purpose: "tools", Approval: "manual", EnvFrom: envFrom, Env: env})
	return s.write(ctx, config)
}

// AddURL creates a private manifest for a Streamable HTTP MCP endpoint.
func (s *MCPConfigStore) AddURL(ctx context.Context, id root.PluginID, endpoint string) error {
	if _, err := root.NewPluginID(id.String()); err != nil {
		return err
	}
	if _, err := plugins.NewRegistration(plugins.Manifest{ID: id, URL: endpoint, AllowAll: true}, nil); err != nil {
		return err
	}
	if strings.ContainsRune(endpoint, 0) {
		return errors.New("invalid MCP endpoint")
	}
	dir := filepath.Join(filepath.Dir(s.Path), "plugins")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dir, id.String()+".json")
	data, err := json.MarshalIndent(map[string]any{"manifest_version": 1, "id": id, "url": endpoint, "allow_tools": []string{"*"}}, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	if err = file.Close(); err != nil {
		os.Remove(path)
		return err
	}
	if err = s.AddManifest(ctx, path); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

func (s *MCPConfigStore) write(ctx context.Context, config dto.MCPConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxMCPConfigBytes {
		return errors.New("MCP config exceeds 64 KiB")
	}
	file, err := os.CreateTemp(filepath.Dir(s.Path), ".axlr-mcp-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporary, s.Path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(s.Path))
	if err != nil {
		return err
	}
	defer directory.Close()
	if s.syncDirectory != nil {
		return s.syncDirectory(directory)
	}
	return directory.Sync()
}
