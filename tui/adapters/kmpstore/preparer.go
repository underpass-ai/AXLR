package kmpstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Preparer creates a store the console chose and fills it: the bundle,
// the person's configuration and the guide.
type Preparer struct {
	// CLI runs kmp-mcp for the bundle import and the guide sync; without a
	// binary only the directory is prepared.
	CLI CLI
	// PluginRoot holds guide/guide.requests.json; empty syncs no guide.
	PluginRoot string
	// UserStore is the per-user default whose configuration a new project
	// store copies; empty, or the selected store itself, copies nothing.
	UserStore string
}

// configurationFiles are the store's own settings a new project store copies
// from the person's per-user store, so the project is ready to use the way
// the person already works (decision B7). Indexes, agents and judgements are
// not copied: KMP rebuilds them.
var configurationFiles = []string{"typesafe.json", "rerank.json", "wake-focus.json", "write-relations.json", "judgement-book.json", "curate.json", "ask-judge.json", "write-expansions.json"}

// Prepare creates the selected store when missing: the directory with KMP's
// self-ignoring .gitignore, the checkout's bundle imported into it (import
// needs an empty store, so it comes first), the user store's configuration,
// then the guide. An existing store is left as it is. env is the
// registration's child environment; the store is pinned into it.
func (p Preparer) Prepare(ctx context.Context, selection Selection, env []string) (Report, error) {
	var report Report
	if selection.Path == "" || !filepath.IsAbs(selection.Path) {
		return report, errors.New("an absolute KMP store path is required")
	}
	if Exists(selection.Path) {
		return report, nil
	}
	report.Created = true
	if err := os.MkdirAll(selection.Path, 0o755); err != nil {
		return report, err
	}
	if ignore := filepath.Join(selection.Path, ".gitignore"); !fileExists(ignore) {
		if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
			return report, err
		}
	}
	child := WithStore(env, selection.Path)
	if bundle := filepath.Join(selection.TopLevel, filepath.FromSlash(BundlePath)); selection.TopLevel != "" && p.CLI.Binary != "" && fileExists(bundle) {
		if out, err := p.CLI.run(ctx, []string{"import", bundle}, child); err != nil {
			report.ImportFailed = err.Error()
		} else {
			report.Imported = importedEvents(out)
		}
	}
	if p.UserStore != "" && filepath.Clean(p.UserStore) != filepath.Clean(selection.Path) {
		for _, name := range configurationFiles {
			source := filepath.Join(p.UserStore, name)
			if !fileExists(source) {
				continue
			}
			if err := copyFile(source, filepath.Join(selection.Path, name)); err != nil {
				return report, err
			}
			report.Configured = append(report.Configured, name)
		}
	}
	if guide := (GuideSync{CLI: p.CLI, PluginRoot: p.PluginRoot}); guide.Configured() {
		if err := guide.Sync(ctx, selection, env); err != nil {
			return report, err
		}
		report.GuideSynced = true
	}
	return report, nil
}

// importedEvents reads kmp-mcp import's receipt, {"events_imported":N} on
// its last line; -1 when it is not there.
func importedEvents(out []byte) int {
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var receipt struct {
			Events *int `json:"events_imported"`
		}
		if json.Unmarshal(bytes.TrimSpace(lines[i]), &receipt) == nil && receipt.Events != nil {
			return *receipt.Events
		}
	}
	return -1
}
