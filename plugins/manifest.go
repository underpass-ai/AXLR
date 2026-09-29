package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/domain"
)

const maxManifestBytes = 64 << 10

type Manifest struct {
	ID         domain.PluginID
	Command    string
	Args       []string
	AllowTools []domain.PluginToolName
}

func LoadManifest(path string) (Manifest, error) {
	if !filepath.IsAbs(path) {
		return Manifest{}, errors.New("plugin manifest path must be absolute")
	}
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxManifestBytes+1))
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxManifestBytes || !utf8.Valid(data) {
		return Manifest{}, errors.New("plugin manifest exceeds 64 KiB or contains invalid UTF-8")
	}
	fields, err := parseManifestFields(data)
	if err != nil {
		return Manifest{}, err
	}
	var version int
	if err := json.Unmarshal(fields["manifest_version"], &version); err != nil || version != 1 {
		return Manifest{}, errors.New("unsupported plugin manifest version")
	}
	var rawID, command string
	var args []string
	var rawArgs []json.RawMessage
	var allow []string
	if err := json.Unmarshal(fields["id"], &rawID); err != nil {
		return Manifest{}, err
	}
	if err := json.Unmarshal(fields["command"], &command); err != nil {
		return Manifest{}, err
	}
	if err := json.Unmarshal(fields["args"], &rawArgs); err != nil {
		return Manifest{}, err
	}
	if rawArgs == nil {
		return Manifest{}, errors.New("plugin args must be an array")
	}
	args = make([]string, 0, len(rawArgs))
	for _, raw := range rawArgs {
		if len(raw) == 0 || raw[0] != '"' {
			return Manifest{}, errors.New("plugin arguments must be strings")
		}
		var arg string
		if err := json.Unmarshal(raw, &arg); err != nil {
			return Manifest{}, err
		}
		args = append(args, arg)
	}
	if err := json.Unmarshal(fields["allow_tools"], &allow); err != nil {
		return Manifest{}, err
	}
	id, err := domain.NewPluginID(rawID)
	if err != nil {
		return Manifest{}, err
	}
	if !filepath.IsAbs(command) || strings.ContainsRune(command, 0) || len(allow) == 0 {
		return Manifest{}, errors.New("plugin requires an absolute command, args and nonempty allow_tools")
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return Manifest{}, errors.New("plugin argument contains NUL")
		}
	}
	seen := map[domain.PluginToolName]bool{}
	tools := make([]domain.PluginToolName, 0, len(allow))
	for _, raw := range allow {
		name, err := domain.NewPluginToolName(raw)
		if err != nil || seen[name] {
			return Manifest{}, errors.New("invalid or duplicate allowed plugin tool")
		}
		seen[name] = true
		tools = append(tools, name)
	}
	return Manifest{ID: id, Command: command, Args: args, AllowTools: tools}, nil
}

func parseManifestFields(data []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("plugin manifest must be a JSON object")
	}
	allowed := map[string]bool{"manifest_version": true, "id": true, "command": true, "args": true, "allow_tools": true}
	fields := make(map[string]json.RawMessage, len(allowed))
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok || !allowed[key] {
			return nil, fmt.Errorf("unknown plugin manifest field %q", key)
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("duplicate plugin manifest field %q", key)
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, errors.New("plugin manifest has trailing JSON")
	}
	for key := range allowed {
		if _, exists := fields[key]; !exists {
			return nil, fmt.Errorf("missing plugin manifest field %q", key)
		}
	}
	return fields, nil
}
