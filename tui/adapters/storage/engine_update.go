package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

func (s *MCPConfigStore) EngineTargets(ctx context.Context) ([]application.EngineUpdateTarget, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	config, err := readMCPConfig(s.Path)
	if err != nil {
		return nil, err
	}
	var targets []application.EngineUpdateTarget
	seen := map[string]bool{}
	for _, entry := range config.Plugins {
		manifest, err := plugins.LoadManifest(entry.Manifest)
		if err != nil {
			return nil, err
		}
		id := manifest.ID.String()
		if id != "made" && id != "kmp" {
			continue
		}
		if seen[id] {
			return nil, errors.New("duplicate engine registration")
		}
		seen[id] = true
		command := manifest.Command
		// Remote engines and custom argument lists are maintained by their owner.
		if manifest.URL != "" || len(manifest.Args) != 0 {
			command = ""
		}
		targets = append(targets, application.EngineUpdateTarget{Engine: id, Manifest: entry.Manifest, Command: command})
	}
	return targets, nil
}

// ActivateEngine switches only the manifest reference under the config lock.
// Original manifests, explicit environments, approvals and stores are preserved.
func (s *MCPConfigStore) ActivateEngine(ctx context.Context, target application.EngineUpdateTarget, launcher string) error {
	if target.Engine != "made" && target.Engine != "kmp" {
		return errors.New("unsupported engine")
	}
	if !filepath.IsAbs(launcher) {
		return errors.New("engine launcher must be absolute")
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
	found := -1
	for i, entry := range config.Plugins {
		manifest, err := plugins.LoadManifest(entry.Manifest)
		if err != nil {
			return err
		}
		if manifest.ID.String() != target.Engine {
			continue
		}
		if found >= 0 || entry.Manifest != target.Manifest || manifest.Command != target.Command || len(manifest.Args) != 0 {
			return errors.New("engine configuration changed; retry /update")
		}
		found = i
	}
	if found < 0 {
		return errors.New("engine is no longer configured")
	}
	original, err := plugins.LoadManifest(target.Manifest)
	if err != nil {
		return err
	}
	allow := []string{"*"}
	if !original.AllowAll {
		allow = nil
		for _, name := range original.AllowTools {
			allow = append(allow, string(name))
		}
	}
	path, err := writeEngineManifest(filepath.Join(filepath.Dir(s.Path), "engines"), target.Engine, launcher, allow)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			os.Remove(path)
		}
	}()
	config.Plugins[found].Manifest = path
	if err := s.write(ctx, config); err != nil {
		// A directory sync can fail after the durable reference was replaced.
		// Keep its target in that case, so the registration never becomes dangling.
		current, readErr := readMCPConfig(s.Path)
		if readErr == nil {
			for _, e := range current.Plugins {
				if e.Manifest == path {
					committed = true
				}
			}
		}
		return err
	}
	committed = true
	return nil
}

// RegisterEngine connects an engine package that mcp.json lacks: it writes
// the engine's manifest beside the others and appends its entry under the
// config lock. An existing registration is never replaced.
func (s *MCPConfigStore) RegisterEngine(ctx context.Context, registration application.EngineRegistration) error {
	if registration.Engine != "made" && registration.Engine != "kmp" {
		return errors.New("unsupported engine")
	}
	if !filepath.IsAbs(registration.Launcher) {
		return errors.New("engine launcher must be absolute")
	}
	for key, source := range registration.EnvFrom {
		if !mcpEnvironmentName.MatchString(key) || !mcpEnvironmentName.MatchString(source) || key == "OPENROUTER_API_KEY" || source == "OPENROUTER_API_KEY" {
			return errors.New("invalid MCP environment source")
		}
	}
	for key, value := range registration.Env {
		if _, resolved := registration.EnvFrom[key]; resolved || !mcpEnvironmentName.MatchString(key) || key == "OPENROUTER_API_KEY" || strings.ContainsRune(value, 0) {
			return errors.New("invalid MCP environment value")
		}
	}
	profile := domain.PluginProfile{ID: root.PluginID(registration.Engine), Name: root.Text(registration.Name), Description: root.Text(registration.Description), Purpose: domain.PluginPurpose(registration.Purpose), Approval: domain.ApprovalMode(registration.Approval)}
	if err := profile.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
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
	for _, entry := range config.Plugins {
		manifest, err := plugins.LoadManifest(entry.Manifest)
		if err != nil {
			return errors.New("cannot validate configured MCP server")
		}
		if manifest.ID.String() == registration.Engine {
			return errors.New("engine is already configured")
		}
	}
	if len(config.Plugins) >= 32 {
		return errors.New("MCP server limit reached")
	}
	path, err := writeEngineManifest(filepath.Join(filepath.Dir(s.Path), "engines"), registration.Engine, registration.Launcher, []string{"*"})
	if err != nil {
		return err
	}
	config.Version = 1
	config.Plugins = append(config.Plugins, dto.MCPPluginConfig{
		Manifest: path, Name: registration.Name, Description: registration.Description,
		Purpose: registration.Purpose, Approval: registration.Approval,
		Env: registration.Env, EnvFrom: registration.EnvFrom,
	})
	if err := s.write(ctx, config); err != nil {
		if current, readErr := readMCPConfig(s.Path); readErr == nil {
			for _, entry := range current.Plugins {
				if entry.Manifest == path {
					return err
				}
			}
		}
		os.Remove(path)
		return err
	}
	return nil
}

// writeEngineManifest writes a private manifest for an engine launcher and
// returns its path; the name is unique, so a running process keeps its own.
func writeEngineManifest(dir, engine, launcher string, allow []string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, engine+"-*.json")
	if err != nil {
		return "", err
	}
	path := file.Name()
	committed := false
	defer func() {
		file.Close()
		if !committed {
			os.Remove(path)
		}
	}()
	data, err := json.MarshalIndent(map[string]any{"manifest_version": 1, "id": engine, "command": launcher, "args": []string{}, "allow_tools": allow}, "", "  ")
	if err != nil {
		return "", err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if _, err := plugins.LoadManifest(path); err != nil {
		return "", err
	}
	committed = true
	return path, nil
}
