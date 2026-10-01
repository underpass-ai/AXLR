package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/application"
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
	dir := filepath.Join(filepath.Dir(s.Path), "engines")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, target.Engine+"-*.json")
	if err != nil {
		return err
	}
	path := file.Name()
	committed := false
	defer func() {
		file.Close()
		if !committed {
			os.Remove(path)
		}
	}()
	data, err := json.MarshalIndent(map[string]any{"manifest_version": 1, "id": target.Engine, "command": launcher, "args": []string{}, "allow_tools": allow}, "", "  ")
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := plugins.LoadManifest(path); err != nil {
		return err
	}
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
