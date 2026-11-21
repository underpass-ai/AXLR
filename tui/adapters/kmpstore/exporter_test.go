package kmpstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// committedBundle writes a checkout's .kmp/memory.jsonl whose header names
// abouts, as kmp-mcp export writes it.
func committedBundle(t *testing.T, top string, abouts ...string) string {
	t.Helper()
	path := filepath.Join(top, filepath.FromSlash(BundlePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	header := `{"bundle_format":3,"event_format":2,"event_count":2,"abouts":["` + strings.Join(abouts, `","`) + `"]}` + "\n"
	if err := os.WriteFile(path, []byte(header+`{"event":1}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A committed bundle that holds a teammate's about besides the project's:
// the store takes each in first (one about at a time; the project's own,
// which the store is ahead on, is refused and that is fine), then both are
// exported, so /memory save never drops the teammate's memory (KMP 1.0
// refuses an export that would) and never passes --overwrite-committed.
func TestExportKeepsEveryAboutTheCommittedBundleHolds(t *testing.T) {
	top, _ := checkout(t)
	bundle := committedBundle(t, top, "project:other", "project:AXLR")
	var runs [][]string
	e := Exporter{CLI: CLI{Binary: "/opt/kmp/bin/kmp-mcp", Run: func(_ context.Context, _ string, args, _ []string) ([]byte, error) {
		runs = append(runs, args)
		if args[0] == "export" {
			return []byte(`{"abouts":["project:AXLR","project:other"],"event_count":5}` + "\n"), nil
		}
		if args[len(args)-1] == "project:AXLR" {
			return []byte("import refused; nothing was written. The destination is ahead"), errors.New("exit status 2")
		}
		return []byte(`{"imported":["project:other"]}` + "\n"), nil
	}}}
	path, events, err := e.Export(context.Background(), Selection{Path: filepath.Join(top, ".kernel"), TopLevel: top}, "project:AXLR", nil)
	if err != nil || path != bundle || events != 5 {
		t.Fatalf("%q %d %v", path, events, err)
	}
	want := [][]string{
		{"import", "--from", bundle, "--about", "project:other"},
		{"import", "--from", bundle, "--about", "project:AXLR"},
		{"export", bundle, "--about", "project:AXLR", "--about", "project:other"},
	}
	if !reflect.DeepEqual(runs, want) {
		t.Fatalf("runs %v", runs)
	}
	for _, args := range runs {
		if strings.Contains(strings.Join(args, " "), "overwrite") {
			t.Fatalf("overwrite passed: %v", args)
		}
	}
}

// Without a committed bundle the save is one export of the session's about.
func TestExportWithoutACommittedBundleExportsTheAboutAlone(t *testing.T) {
	top, _ := checkout(t)
	var runs [][]string
	e := Exporter{CLI: CLI{Binary: "/opt/kmp/bin/kmp-mcp", Run: func(_ context.Context, _ string, args, _ []string) ([]byte, error) {
		runs = append(runs, args)
		return []byte(`{"event_count":1}`), nil
	}}}
	if _, _, err := e.Export(context.Background(), Selection{Path: filepath.Join(top, ".kernel"), TopLevel: top}, "project:AXLR", nil); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || !reflect.DeepEqual(runs[0], []string{"export", filepath.Join(top, filepath.FromSlash(BundlePath)), "--about", "project:AXLR"}) {
		t.Fatalf("runs %v", runs)
	}
}

// KMP's refusal reaches the person whole, past the six lines a failed verb
// keeps, and nothing is written.
func TestExportSurfacesKMPsRefusal(t *testing.T) {
	top, _ := checkout(t)
	committedBundle(t, top, "project:other")
	refusal := "kmp-mcp export: refused: the export would drop 3 events of project:other that " + BundlePath + " holds.\nWays out:\n1. export without --about\n2. kmp-mcp import --from .kmp/memory.jsonl --about project:other\n3. --overwrite-committed\n4. a\n5. b\n6. c\n7. d"
	e := Exporter{CLI: CLI{Binary: "/opt/kmp/bin/kmp-mcp", Run: func(_ context.Context, _ string, args, _ []string) ([]byte, error) {
		if args[0] == "export" {
			return []byte(refusal), errors.New("exit status 2")
		}
		return []byte("import refused; different history for project:other"), errors.New("exit status 2")
	}}}
	_, _, err := e.Export(context.Background(), Selection{Path: filepath.Join(top, ".kernel"), TopLevel: top}, "project:AXLR", nil)
	if err == nil || !strings.Contains(err.Error(), "would drop 3 events of project:other") || !strings.Contains(err.Error(), "7. d") || !strings.Contains(err.Error(), "nothing was written") || !strings.Contains(err.Error(), "project:other: import refused; different history") {
		t.Fatalf("%v", err)
	}
	if got := refusalText(nil, errors.New("exit status 2")); got != "exit status 2" {
		t.Fatal(got)
	}
	if got := refusalText([]byte(strings.Repeat("é", 2000)), nil); len(got) > 2048+len("…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("%d", len(got))
	}
}
