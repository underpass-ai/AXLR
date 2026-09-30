package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/buildinfo"
)

func TestVersionFlagNeedsNoProvider(t *testing.T) {
	var output bytes.Buffer
	code := run(context.Background(), []string{"--version"}, func(string) string { return "" }, func(tea.Model) error { t.Fatal("launched UI"); return nil }, &output)
	if code != 0 || strings.TrimSpace(output.String()) != buildinfo.Version {
		t.Fatalf("version: %d %q", code, output.String())
	}
}
