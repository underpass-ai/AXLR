package kmpstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Exporter writes an about of a store as the checkout's bundle.
type Exporter struct{ CLI CLI }

// Export writes the store's events under one about as the checkout's
// bundle (/memory save, decision I3 of the lifecycle integration design):
// kmp-mcp export <bundle> --about <about>, which a live engine on the
// store does not block and which replaces the file (checked on 13 October
// 2026). It returns the bundle's path and how many events it holds.
func (e Exporter) Export(ctx context.Context, selection Selection, about string, env []string) (string, int, error) {
	if e.CLI.Binary == "" {
		return "", 0, errNoBinary
	}
	if selection.TopLevel == "" {
		return "", 0, errors.New("the workspace is not a git checkout: the bundle lives at <checkout>/" + BundlePath)
	}
	if strings.TrimSpace(about) == "" {
		return "", 0, errors.New("an about is required")
	}
	bundle := filepath.Join(selection.TopLevel, filepath.FromSlash(BundlePath))
	if err := os.MkdirAll(filepath.Dir(bundle), 0o755); err != nil {
		return "", 0, err
	}
	output, err := e.CLI.run(ctx, []string{"export", bundle, "--about", about}, WithStore(env, selection.Path))
	if err != nil {
		return "", 0, err
	}
	return bundle, exportedEvents(output), nil
}

// exportedEvents reads event_count from the JSON line kmp-mcp export
// prints last; -1 when it printed none.
func exportedEvents(output []byte) int {
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var report struct {
			EventCount int `json:"event_count"`
		}
		if json.Unmarshal(line, &report) == nil {
			return report.EventCount
		}
	}
	return -1
}
