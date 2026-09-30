package storage

import (
	"context"
	"encoding/json"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/domain"
	"os"
	"path/filepath"
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
