package storage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func portableManifest(body []byte) []byte {
	if runtime.GOOS != "windows" {
		return body
	}
	command, _ := json.Marshal(os.Args[0])
	return bytes.ReplaceAll(body, []byte(`"/bin/echo"`), command)
}

func TestLoadMCPConfigRegistersPersistentPluginWithSelectedEnvironment(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "kmp-manifest.json")
	if err := os.WriteFile(manifest, portableManifest([]byte(`{"manifest_version":1,"id":"kmp","command":"/bin/echo","args":[],"allow_tools":["*"]}`)), 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "mcp.json")
	data, _ := json.Marshal(map[string]any{"version": 1, "plugins": []any{map[string]any{"manifest": manifest, "env": map[string]string{"MODE": "embedded"}, "env_from": map[string]string{"ACCESS_TOKEN": "SELECTED_TOKEN"}}}})
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	registrations, err := LoadMCPConfig(config, func(key string) string {
		if key == "SELECTED_TOKEN" {
			return "selected"
		}
		return "must-not-leak"
	})
	if err != nil || len(registrations) != 1 || !registrations[0].Manifest.AllowAll {
		t.Fatalf("registrations: %+v %v", registrations, err)
	}
	if strings.Join(registrations[0].Env, ",") != "ACCESS_TOKEN=selected,MODE=embedded" {
		t.Fatalf("wrong plugin environment: %v", registrations[0].Env)
	}
	if none, err := LoadMCPConfig(filepath.Join(dir, "absent.json"), func(string) string { return "" }); err != nil || len(none) != 0 {
		t.Fatalf("missing config: %+v %v", none, err)
	}
}

func TestLoadMCPConfigRejectsUnsafeOrInvalidFile(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "m.json")
	if err := os.WriteFile(manifest, portableManifest([]byte(`{"manifest_version":1,"id":"plugin","command":"/bin/echo","args":[],"allow_tools":["read"]}`)), 0600); err != nil {
		t.Fatal(err)
	}
	valid, _ := json.Marshal(map[string]any{"version": 1, "plugins": []any{map[string]any{"manifest": manifest}}})
	config := filepath.Join(dir, "mcp.json")
	for name, body := range map[string]string{
		"unknown":       strings.Replace(string(valid), `"version":1`, `"extra":1,"version":1`, 1),
		"version":       strings.Replace(string(valid), `"version":1`, `"version":2`, 1),
		"secret source": `{"version":1,"plugins":[{"manifest":"` + manifest + `","env_from":{"TOKEN":"OPENROUTER_API_KEY"}}]}`,
		"invalid env":   `{"version":1,"plugins":[{"manifest":"` + manifest + `","env":{"A=B":"x"}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(config, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadMCPConfig(config, func(string) string { return "secret" }); err == nil {
				t.Fatal("unsafe config accepted")
			}
		})
	}
	if err := os.WriteFile(config, valid, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(config, 0644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if _, err := LoadMCPConfig(config, func(string) string { return "" }); err == nil {
			t.Fatal("public config accepted")
		}
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(manifest, config); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMCPConfig(config, func(string) string { return "" }); err == nil {
		t.Fatal("symlink config accepted")
	}
}

func TestLoadMCPConfigErrorNamesPathAndSpecificReason(t *testing.T) {
	dir := t.TempDir()
	none := func(string) string { return "" }
	expect := func(t *testing.T, path, reason string) {
		t.Helper()
		_, err := LoadMCPConfig(path, none)
		if err == nil {
			t.Fatalf("%s accepted", reason)
		}
		if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), reason) {
			t.Fatalf("error %q must name %s and say %q", err, path, reason)
		}
	}
	t.Run("not a regular file", func(t *testing.T) {
		expect(t, dir, "is not a regular file")
	})
	t.Run("size", func(t *testing.T) {
		path := filepath.Join(dir, "large.json")
		if err := os.WriteFile(path, make([]byte, maxMCPConfigBytes+1), 0600); err != nil {
			t.Fatal(err)
		}
		expect(t, path, "bytes; the limit is")
	})
	if runtime.GOOS != "windows" {
		t.Run("mode", func(t *testing.T) {
			path := filepath.Join(dir, "public.json")
			if err := os.WriteFile(path, []byte(`{"version":1,"plugins":[]}`), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0644); err != nil {
				t.Fatal(err)
			}
			expect(t, path, "has mode 0644")
			expect(t, path, "chmod 600 "+path)
		})
	}
}

func TestLoadMCPConfigRejectsFIFOWithoutBlocking(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX FIFO")
	}
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := makeFIFO(path); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := LoadMCPConfig(path, func(string) string { return "" }); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO config accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("config reader blocked opening FIFO")
	}
}

func TestLoadMCPConfigRejectsDirectoryAndOversizedFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadMCPConfig(dir, func(string) string { return "" }); err == nil {
		t.Fatal("directory config accepted")
	}
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, make([]byte, maxMCPConfigBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMCPConfig(path, func(string) string { return "" }); err == nil {
		t.Fatal("oversized config accepted")
	}
}
