package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

const maxCatalogBytes = 16 << 20

var pluginSelector = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*@[A-Za-z0-9][A-Za-z0-9._-]*$`)

// PluginCatalog uses the public Codex CLI. No Codex state files are edited.
type PluginCatalog struct {
	Binary string
	Run    func(context.Context, string, ...string) ([]byte, error)
}

func (c PluginCatalog) execute(ctx context.Context, args ...string) ([]byte, error) {
	limit := 45 * time.Second
	if len(args) > 1 && args[1] != "list" {
		limit = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	if c.Run != nil {
		return c.Run(ctx, c.Binary, args...)
	}
	binary := c.Binary
	if binary == "" {
		binary = "codex"
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.New("Codex CLI is unavailable")
		}
		return nil, fmt.Errorf("Codex plugin command failed: %w", err)
	}
	if len(output) > maxCatalogBytes {
		return nil, errors.New("Codex plugin catalog is too large")
	}
	return output, nil
}

func (c PluginCatalog) List(ctx context.Context, available bool) ([]application.InstalledPlugin, error) {
	args := []string{"plugin", "list"}
	if available {
		args = append(args, "--available")
	}
	args = append(args, "--json")
	output, err := c.execute(ctx, args...)
	if err != nil {
		return nil, err
	}
	var data struct {
		Installed []pluginEntry `json:"installed"`
		Available []pluginEntry `json:"available"`
	}
	if err := json.Unmarshal(output, &data); err != nil {
		return nil, errors.New("invalid Codex plugin catalog")
	}
	entries := data.Installed
	if available {
		entries = append(entries, data.Available...)
	}
	result := make([]application.InstalledPlugin, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		if !pluginSelector.MatchString(entry.ID) || seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		item := application.InstalledPlugin{ID: entry.ID, Name: entry.Name, Marketplace: entry.Marketplace, Version: entry.Version, Source: entry.MarketplaceSource.Source, InstallPolicy: entry.InstallPolicy, AuthPolicy: entry.AuthPolicy, Installed: entry.Installed, Enabled: entry.Enabled}
		if entry.Installed && entry.Source.Source == "local" {
			item.Description, item.Components = readComponents(entry.Source.Path)
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		rank := func(id string) int {
			switch id {
			case "kmp@underpass":
				return 0
			case "made@made":
				return 1
			}
			return 2
		}
		ri, rj := rank(result[i].ID), rank(result[j].ID)
		if ri != rj {
			return ri < rj
		}
		if result[i].Installed != result[j].Installed {
			return result[i].Installed
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

type pluginEntry struct {
	ID            string `json:"pluginId"`
	Name          string `json:"name"`
	Marketplace   string `json:"marketplaceName"`
	Version       string `json:"version"`
	Installed     bool   `json:"installed"`
	Enabled       bool   `json:"enabled"`
	InstallPolicy string `json:"installPolicy"`
	AuthPolicy    string `json:"authPolicy"`
	Source        struct {
		Source string `json:"source"`
		Path   string `json:"path"`
	} `json:"source"`
	MarketplaceSource struct {
		Source string `json:"source"`
	} `json:"marketplaceSource"`
}

func readComponents(root string) (string, []string) {
	if !filepath.IsAbs(root) {
		return "", nil
	}
	file, err := os.Open(filepath.Join(root, ".codex-plugin", "plugin.json"))
	if err != nil {
		return "", nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64<<10))
	if err != nil {
		return "", nil
	}
	var manifest struct {
		Description string          `json:"description"`
		Skills      json.RawMessage `json:"skills"`
		MCP         json.RawMessage `json:"mcpServers"`
		Apps        json.RawMessage `json:"apps"`
		Hooks       json.RawMessage `json:"hooks"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return "", nil
	}
	var components []string
	for _, entry := range []struct {
		name string
		raw  json.RawMessage
	}{{"skills", manifest.Skills}, {"mcp", manifest.MCP}, {"apps", manifest.Apps}, {"hooks", manifest.Hooks}} {
		if len(entry.raw) > 0 && string(entry.raw) != "null" && string(entry.raw) != "{}" && string(entry.raw) != "[]" && string(entry.raw) != `""` {
			components = append(components, entry.name)
		}
	}
	return manifest.Description, components
}

func (c PluginCatalog) Install(ctx context.Context, id string) error { return c.change(ctx, "add", id) }
func (c PluginCatalog) AddMarketplace(ctx context.Context, source string) error {
	if source == "" || strings.HasPrefix(source, "-") || strings.ContainsAny(source, "\x00\n\r") {
		return errors.New("invalid marketplace source")
	}
	_, err := c.execute(ctx, "plugin", "marketplace", "add", source, "--json")
	return err
}
func (c PluginCatalog) change(ctx context.Context, action, id string) error {
	if !pluginSelector.MatchString(id) || strings.ContainsRune(id, 0) {
		return errors.New("invalid plugin selector")
	}
	_, err := c.execute(ctx, "plugin", action, id, "--json")
	return err
}
