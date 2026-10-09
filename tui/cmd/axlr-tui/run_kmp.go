package main

import (
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/plugins"
)

// kmpGuideRoot is the plugin directory of the KMP engine AXLR installed,
// <root>/scripts/run-embedded-mcp.sh, when it holds the guide assets a
// store without a guide is synced from; empty otherwise.
func kmpGuideRoot(registrations []plugins.Registration) string {
	for _, r := range registrations {
		if r.Manifest.ID != "kmp" || r.Manifest.Command == "" {
			continue
		}
		root := filepath.Dir(filepath.Dir(r.Manifest.Command))
		if info, err := os.Stat(filepath.Join(root, "guide", "guide.requests.json")); err == nil && info.Mode().IsRegular() {
			return root
		}
	}
	return ""
}
