package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/buildinfo"
)

func TestVersionFlagNeedsNoWorkspace(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr); code != 0 || strings.TrimSpace(stdout.String()) != buildinfo.Version || stderr.Len() != 0 {
		t.Fatalf("version: %d %q %q", code, stdout.String(), stderr.String())
	}
}
