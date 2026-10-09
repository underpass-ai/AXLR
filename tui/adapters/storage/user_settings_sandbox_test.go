package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExecSandboxSettings(t *testing.T) {
	base := UserSettings{Language: "en", Theme: "auto", Icons: "safe"}
	if got := base.ExecSandbox(); got.Mode != "off" || !got.AllowsNetwork() {
		t.Fatalf("default = %+v", got)
	}
	off := false
	base.Sandbox = &ExecSandboxSettings{Mode: "required", Network: &off, Writable: []string{t.TempDir()}}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := base.ExecSandbox(); got.Mode != "required" || got.AllowsNetwork() || len(got.Writable) != 1 {
		t.Fatalf("configured = %+v", got)
	}
	base.Sandbox = &ExecSandboxSettings{Writable: []string{"/a"}}
	if got := base.ExecSandbox(); got.Mode != "off" {
		t.Fatalf("mode without a value = %q", got.Mode)
	}
	many := make([]string, maxSandboxWritable+1)
	for i := range many {
		many[i] = "/w"
	}
	for _, tc := range []struct {
		sandbox ExecSandboxSettings
		want    string
	}{
		{ExecSandboxSettings{Mode: "on"}, "mode must be off, auto or required"},
		{ExecSandboxSettings{Mode: "auto", Writable: []string{"relative/cache"}}, "clean absolute path"},
		{ExecSandboxSettings{Mode: "auto", Writable: []string{string(filepath.Separator)}}, "clean absolute path"},
		{ExecSandboxSettings{Mode: "auto", Writable: []string{"/tmp/../etc"}}, "clean absolute path"},
		{ExecSandboxSettings{Mode: "auto", Writable: many}, "more than 32 paths"},
	} {
		settings := base
		sandbox := tc.sandbox
		settings.Sandbox = &sandbox
		if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%+v: %v", tc.sandbox, err)
		}
	}
}
