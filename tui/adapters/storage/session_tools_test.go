package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// bigTools is a tool set the size of the console's (one 300 KB schema).
func bigTools(t *testing.T, name string) []domain.AvailableTool {
	t.Helper()
	identity, err := domain.NewLocalToolIdentity("read")
	must(t, err)
	params, err := root.NewJSONObject([]byte(`{"type":"object","description":"` + strings.Repeat("x", 300<<10) + `"}`))
	must(t, err)
	return []domain.AvailableTool{{Identity: identity, Definition: root.ToolDefinition{Name: root.ToolName(name), Description: "read a file", Parameters: params}}}
}

func sessionWith(t *testing.T, id string, tools []domain.AvailableTool) domain.Session {
	t.Helper()
	s, err := domain.NewSession(domain.SessionID(id), domain.Workspace(t.TempDir()), "test/model")
	must(t, err)
	must(t, s.BeginTurn("hello", tools))
	must(t, s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "hi"}}))
	return s
}

// On 14 October 2026 an 8-message session file weighed 329 KB, 338 KB of
// it tool schemas: two sessions with the same tools now share one stored
// tool set, each session file names it and stays small, and a load gives
// back the same tools.
func TestSessionsNameOneStoredToolSetInsteadOfCarryingIt(t *testing.T) {
	store, dir := openStore(t)
	tools := bigTools(t, "local_read")
	a := sessionWith(t, "0123456789abcdef0123456789abcdef", tools)
	b := sessionWith(t, "fedcba9876543210fedcba9876543210", tools)
	must(t, store.Save(context.Background(), a))
	must(t, store.Save(context.Background(), b))
	for _, s := range []domain.Session{a, b} {
		raw, err := os.ReadFile(filepath.Join(dir, string(s.Export().ID)+".json"))
		must(t, err)
		if len(raw) > 8<<10 || strings.Contains(string(raw), `"tool_snapshot":`) || !strings.Contains(string(raw), `"tool_snapshot_ref":"`) {
			t.Fatalf("session file of %d bytes: %.300s", len(raw), raw)
		}
	}
	sets, err := os.ReadDir(filepath.Join(dir, toolsDirName))
	must(t, err)
	if len(sets) != 1 {
		t.Fatalf("tool sets %v", sets)
	}
	reopened, err := New(dir)
	must(t, err)
	defer reopened.Close()
	must(t, store.Release(a.Export().ID))
	got, err := reopened.Load(context.Background(), a.Export().ID)
	must(t, err)
	if !reflect.DeepEqual(got.ToolSnapshot(), a.ToolSnapshot()) {
		t.Fatal("the stored tool set did not give back the session's tools")
	}
}

// A session file written before 14 October 2026 carries its tools and
// still loads; a session bundle keeps carrying them, since it is read on
// another machine.
func TestOlderSessionFilesAndBundlesCarryTheirTools(t *testing.T) {
	store, dir := openStore(t)
	s := sessionWith(t, "0123456789abcdef0123456789abcdef", bigTools(t, "local_read"))
	record, err := snapshot(s)
	must(t, err)
	data, err := json.Marshal(record)
	must(t, err)
	if record.Version != snapshotVersion || !strings.Contains(string(data), `"tool_snapshot":[`) {
		t.Fatalf("version %d", record.Version)
	}
	must(t, os.WriteFile(filepath.Join(dir, record.ID+".json"), data, 0600))
	got, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	if !reflect.DeepEqual(got.ToolSnapshot(), s.ToolSnapshot()) {
		t.Fatal("a version 2 file lost its tools")
	}
	bundle, err := SessionBundles{}.Encode(s, application.SessionBundleMeta{ExportedAt: time.Unix(1, 0)})
	must(t, err)
	var decoded sessionBundle
	must(t, json.Unmarshal(bundle, &decoded))
	if decoded.Snapshot.ToolSnapshotRef != "" || len(decoded.Snapshot.ToolSnapshot) != 1 {
		t.Fatal("a bundle must carry its tools")
	}
}

// A stored tool set that no longer hashes to its name is refused rather
// than offered to the model.
func TestAChangedToolSetIsRefused(t *testing.T) {
	store, dir := openStore(t)
	s := sessionWith(t, "0123456789abcdef0123456789abcdef", bigTools(t, "local_read"))
	must(t, store.Save(context.Background(), s))
	sets, err := os.ReadDir(filepath.Join(dir, toolsDirName))
	must(t, err)
	path := filepath.Join(dir, toolsDirName, sets[0].Name())
	data, err := os.ReadFile(path)
	must(t, err)
	must(t, os.WriteFile(path, []byte(strings.Replace(string(data), "read a file", "rm -rf /", 1)), 0600))
	reopened, err := New(dir)
	must(t, err)
	defer reopened.Close()
	must(t, store.Release(s.Export().ID))
	if _, err := reopened.Load(context.Background(), s.Export().ID); err == nil || !strings.Contains(err.Error(), "does not match its name") {
		t.Fatalf("load %v", err)
	}
}

// Deleting a session removes a tool set no other session names, once it
// is older than the grace another console may need; a set still named, or
// just written, stays.
func TestDeletingASessionSweepsToolSetsNoSessionNames(t *testing.T) {
	store, dir := openStore(t)
	kept := sessionWith(t, "0123456789abcdef0123456789abcdef", bigTools(t, "local_read"))
	gone := sessionWith(t, "fedcba9876543210fedcba9876543210", bigTools(t, "local_list"))
	must(t, store.Save(context.Background(), kept))
	must(t, store.Save(context.Background(), gone))
	sets, err := os.ReadDir(filepath.Join(dir, toolsDirName))
	must(t, err)
	if len(sets) != 2 {
		t.Fatalf("tool sets %v", sets)
	}
	old := time.Now().Add(-2 * toolSetGrace)
	for _, set := range sets {
		must(t, os.Chtimes(filepath.Join(dir, toolsDirName, set.Name()), old, old))
	}
	must(t, store.Delete(context.Background(), gone.Export().ID))
	left, err := os.ReadDir(filepath.Join(dir, toolsDirName))
	must(t, err)
	if len(left) != 1 {
		t.Fatalf("tool sets after the delete %v", left)
	}
	if _, err := store.Load(context.Background(), kept.Export().ID); err != nil {
		t.Fatalf("the kept session lost its tools: %v", err)
	}
}
