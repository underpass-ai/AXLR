package axlrplugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func loadManifest(root string) (manifest, error) {
	var m manifest
	data, err := os.ReadFile(filepath.Join(root, ".codex-plugin", "plugin.json"))
	if err != nil {
		return m, fmt.Errorf("Codex-compatible .codex-plugin/plugin.json: %w", err)
	}
	if len(data) > 64<<10 {
		return m, errors.New("plugin manifest exceeds 64 KiB")
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, err
	}
	m.ID = m.Name
	if !packageID.MatchString(m.ID) {
		return m, errors.New("invalid plugin name")
	}
	if m.Version == "" {
		return m, errors.New("plugin version is required")
	}
	return m, nil
}
func (m manifest) components() []string {
	result := []string{}
	if len(m.Skills) > 0 && string(m.Skills) != "null" {
		result = append(result, "skills")
	}
	if len(m.MCPServers) > 0 && string(m.MCPServers) != "null" || m.MCPManifest != "" {
		result = append(result, "mcp")
	}
	return result
}
func (m manifest) skillRoot(root string) string {
	var rel string
	if err := json.Unmarshal(m.Skills, &rel); err != nil || rel == "" {
		return ""
	}
	path, err := safeChild(root, rel)
	if err != nil {
		return ""
	}
	return path
}
func safeChild(root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", errors.New("absolute plugin component path")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("plugin component escapes package")
	}
	path := filepath.Join(root, clean)
	return path, nil
}

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type mcpDefinition struct {
	Path    string
	EnvFrom map[string]string
	Env     map[string]string
}

type codexServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	URL     string            `json:"url"`
	EnvVars []string          `json:"env_vars"`
	Env     map[string]string `json:"env"`
}

func materializeMCP(root string, m manifest) ([]mcpDefinition, error) {
	if m.MCPManifest != "" {
		path, err := safeChild(root, m.MCPManifest)
		if err != nil {
			return nil, err
		}
		return []mcpDefinition{{Path: path}}, nil
	}
	if len(m.MCPServers) == 0 || string(m.MCPServers) == "null" {
		return nil, nil
	}
	raw := m.MCPServers
	var rel string
	if json.Unmarshal(raw, &rel) == nil {
		path, err := safeChild(root, rel)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(data) > 64<<10 {
			return nil, errors.New("MCP definition exceeds 64 KiB")
		}
		var wrapper struct {
			MCPServers json.RawMessage `json:"mcpServers"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, err
		}
		raw = wrapper.MCPServers
	}
	var servers map[string]codexServer
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, fmt.Errorf("invalid mcpServers: %w", err)
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	paths := make([]mcpDefinition, 0, len(names))
	dir := filepath.Join(root, ".axlr-mcp")
	if len(names) > 0 {
		if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
			return nil, err
		}
	}
	for _, name := range names {
		server := servers[name]
		if !packageID.MatchString(name) {
			return nil, errors.New("invalid MCP server name")
		}
		if server.Command == "" && server.URL == "" || server.Command != "" && server.URL != "" {
			return nil, fmt.Errorf("MCP server %s needs one transport", name)
		}
		id := m.ID + "-" + name
		if !packageID.MatchString(id) {
			return nil, errors.New("MCP ID is too long")
		}
		definition := map[string]any{"manifest_version": 1, "id": id, "allow_tools": []string{"*"}}
		if server.Command != "" {
			command := strings.ReplaceAll(server.Command, "${CLAUDE_PLUGIN_ROOT}", root)
			command = strings.ReplaceAll(command, "${CODEX_PLUGIN_ROOT}", root)
			if !filepath.IsAbs(command) {
				resolved, err := exec.LookPath(command)
				if err != nil {
					return nil, fmt.Errorf("MCP command %s: %w", command, err)
				}
				command = resolved
			}
			definition["command"] = command
			args := make([]string, len(server.Args))
			for i, arg := range server.Args {
				args[i] = strings.ReplaceAll(strings.ReplaceAll(arg, "${CLAUDE_PLUGIN_ROOT}", root), "${CODEX_PLUGIN_ROOT}", root)
			}
			definition["args"] = args
		} else {
			definition["url"] = server.URL
		}
		data, err := json.MarshalIndent(definition, "", "  ")
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
			return nil, err
		}
		envFrom := map[string]string{}
		if server.Command != "" {
			envFrom["HOME"] = "HOME"
			envFrom["PATH"] = "PATH"
		}
		for _, key := range server.EnvVars {
			if !environmentName.MatchString(key) || key == "OPENROUTER_API_KEY" {
				return nil, errors.New("invalid MCP environment variable")
			}
			envFrom[key] = key
		}
		env := map[string]string{}
		for key, value := range server.Env {
			if !environmentName.MatchString(key) || key == "OPENROUTER_API_KEY" {
				return nil, errors.New("invalid MCP environment key")
			}
			if value == "${CLAUDE_PLUGIN_ROOT}" || value == "${CODEX_PLUGIN_ROOT}" {
				env[key] = root
				delete(envFrom, key)
				continue
			}
			if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") && environmentName.MatchString(value[2:len(value)-1]) {
				envFrom[key] = value[2 : len(value)-1]
				continue
			}
			env[key] = strings.ReplaceAll(strings.ReplaceAll(value, "${CLAUDE_PLUGIN_ROOT}", root), "${CODEX_PLUGIN_ROOT}", root)
			delete(envFrom, key)
		}
		paths = append(paths, mcpDefinition{Path: path, EnvFrom: envFrom, Env: env})
	}
	return paths, nil
}
