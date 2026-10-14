package kmpstore

import (
	"os"
	"path/filepath"
)

// Binary is the kmp-mcp executable behind a registration's command and the
// plugin root that holds the guide assets: a package launcher gives both,
// a bare binary gives itself and no root.
func Binary(command string) (binary, pluginRoot string) {
	if filepath.Base(command) != Launcher {
		return command, ""
	}
	root := filepath.Dir(filepath.Dir(command))
	binary = filepath.Join(root, "bin", "kmp-mcp")
	if info, err := os.Stat(filepath.Join(root, "guide", "guide.requests.json")); err == nil && info.Mode().IsRegular() {
		return binary, root
	}
	return binary, ""
}
