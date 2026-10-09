package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneDefaultRemovesOnlyOldTracesAndCapturesAXLRNamed(t *testing.T) {
	state := t.TempDir()
	if err := os.Chmod(state, 0o700); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "XDG_STATE_HOME" {
			return state
		}
		return ""
	}
	dir, err := DefaultDirectory(getenv)
	if err != nil {
		t.Fatal(err)
	}
	at := func(name string) string { return filepath.Join(dir, name) }
	files := []string{
		"trace-1-2.jsonl", "trace-3-4.jsonl", "trace-5-6.jsonl", "mine.jsonl", "notes.txt",
		"trace-1-2.jsonl.payloads-7/000001-request.json", "trace-1-2.jsonl.payloads-7/000001-response.sse",
		"mine.jsonl.payloads-8/000002-response.json", "mine.jsonl.payloads-8/notes.txt",
		"trace-9-9.jsonl.payloads-9/nested/000003-request.json",
		"trace-5-6.jsonl.payloads-1/000004-request.json",
	}
	for _, name := range files {
		if err := os.MkdirAll(filepath.Dir(at(name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(at(name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-40 * 24 * time.Hour)
	for _, name := range append(files, "trace-1-2.jsonl.payloads-7", "mine.jsonl.payloads-8", "trace-9-9.jsonl.payloads-9/nested", "trace-9-9.jsonl.payloads-9", "trace-5-6.jsonl.payloads-1") {
		if name == "trace-3-4.jsonl" {
			continue
		}
		if err := os.Chtimes(at(name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	keep := []string{at("trace-5-6.jsonl"), at("trace-5-6.jsonl.payloads-1")}
	if err := PruneDefault(getenv, 0, time.Now(), keep...); err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if _, err := os.Stat(at(name)); err != nil {
			t.Fatalf("retention 0 deleted %s", name)
		}
	}
	if err := PruneDefault(getenv, 30*24*time.Hour, time.Now(), keep...); err != nil {
		t.Fatal(err)
	}
	for name, gone := range map[string]bool{
		"trace-1-2.jsonl":                                       true,
		"trace-1-2.jsonl.payloads-7":                            true,
		"mine.jsonl.payloads-8/000002-response.json":            true,
		"trace-3-4.jsonl":                                       false,
		"trace-5-6.jsonl":                                       false,
		"trace-5-6.jsonl.payloads-1/000004-request.json":        false,
		"mine.jsonl":                                            false,
		"notes.txt":                                             false,
		"mine.jsonl.payloads-8/notes.txt":                       false,
		"trace-9-9.jsonl.payloads-9/nested/000003-request.json": false,
	} {
		if _, err := os.Lstat(at(name)); os.IsNotExist(err) != gone {
			t.Errorf("%s: deleted=%v, want %v", name, os.IsNotExist(err), gone)
		}
	}
}

func TestPruneDefaultNeverFollowsLinks(t *testing.T) {
	state := t.TempDir()
	if err := os.Chmod(state, 0o700); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "XDG_STATE_HOME" {
			return state
		}
		return ""
	}
	dir, err := DefaultDirectory(getenv)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	target := filepath.Join(outside, "trace-1-1.jsonl")
	capture := filepath.Join(outside, "000001-request.json")
	for _, path := range []string{target, capture} {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{filepath.Join(dir, "trace-2-2.jsonl"): target, filepath.Join(dir, "trace-2-2.jsonl.payloads-3"): outside}
	for link, to := range links {
		if err := os.Symlink(to, link); err != nil {
			t.Skip("symbolic links are unavailable:", err)
		}
	}
	// Judged from a year ahead, every entry is past its retention.
	if err := PruneDefault(getenv, 24*time.Hour, time.Now().Add(365*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{target, capture, filepath.Join(dir, "trace-2-2.jsonl"), filepath.Join(dir, "trace-2-2.jsonl.payloads-3")} {
		if _, err := os.Lstat(path); err != nil {
			t.Errorf("%s was deleted", path)
		}
	}
}
