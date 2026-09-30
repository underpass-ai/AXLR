package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/dto"
)

const maxMCPConfigBytes = 64 << 10

var mcpEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// LoadMCPConfig reads explicit, persistent plugin registrations. A missing
// configuration means no external servers were selected for AXLR.
func LoadMCPConfig(path string, getenv func(string) string) ([]plugins.Registration, error) {
	if !filepath.IsAbs(path) || getenv == nil {
		return nil, errors.New("MCP config path must be absolute")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > maxMCPConfigBytes {
		return nil, errors.New("MCP config must be a private regular file of at most 64 KiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxMCPConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxMCPConfigBytes || !utf8.Valid(data) {
		return nil, errors.New("invalid MCP config size or UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var config dto.MCPConfig
	if err := decoder.Decode(&config); err != nil {
		return nil, errors.New("invalid MCP config JSON")
	}
	if decoder.Decode(new(any)) != io.EOF || config.Version != 1 || len(config.Plugins) > 32 {
		return nil, errors.New("invalid MCP config version or plugin count")
	}
	registrations := make([]plugins.Registration, 0, len(config.Plugins))
	for _, selected := range config.Plugins {
		manifest, err := plugins.LoadManifest(selected.Manifest)
		if err != nil {
			return nil, err
		}
		if len(selected.Env)+len(selected.EnvFrom) > 64 {
			return nil, errors.New("too many MCP plugin environment entries")
		}
		values := make(map[string]string, len(selected.Env)+len(selected.EnvFrom))
		for key, value := range selected.Env {
			if !mcpEnvironmentName.MatchString(key) || key == "OPENROUTER_API_KEY" || strings.ContainsRune(value, 0) {
				return nil, errors.New("invalid MCP plugin environment")
			}
			values[key] = value
		}
		for key, source := range selected.EnvFrom {
			if !mcpEnvironmentName.MatchString(key) || !mcpEnvironmentName.MatchString(source) || source == "OPENROUTER_API_KEY" || key == "OPENROUTER_API_KEY" {
				return nil, errors.New("invalid MCP plugin environment source")
			}
			if _, exists := values[key]; exists {
				return nil, errors.New("duplicate MCP plugin environment key")
			}
			values[key] = getenv(source)
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		env := make([]string, 0, len(keys))
		for _, key := range keys {
			env = append(env, key+"="+values[key])
		}
		registration, err := plugins.NewRegistration(manifest, env)
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, registration)
	}
	return registrations, nil
}
