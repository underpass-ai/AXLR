package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/underpass-ai/AXLR/plugins"
)

func TestKMPGuideRootIsTheEnginesPluginDirectory(t *testing.T) {
	root := t.TempDir()
	command := filepath.Join(root, "scripts", "run-embedded-mcp.sh")
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: "kmp", Command: command, AllowAll: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := kmpGuideRoot([]plugins.Registration{registration}); got != "" {
		t.Fatalf("root without guide assets = %q", got)
	}
	if err := os.MkdirAll(filepath.Join(root, "guide"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "guide", "guide.requests.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := kmpGuideRoot([]plugins.Registration{registration}); got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}
