package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func fixture(t *testing.T) domain.Session {
	t.Helper()
	s, e := domain.NewSession("0123456789abcdef0123456789abcdef", "/workspace", "test/model")
	must(t, e)
	return s
}
func openStore(t *testing.T) (*SessionStore, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "sessions")
	s, e := New(dir)
	must(t, e)
	t.Cleanup(func() { s.Close() })
	return s, dir
}
func TestSessionStoreRoundTripListPermissionsAndNoAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "secret-not-for-disk")
	store, dir := openStore(t)
	s := fixture(t)
	must(t, s.BeginTurn("hello", nil))
	must(t, s.InterruptDraft("partial"))
	must(t, store.Save(context.Background(), s))
	got, e := store.Load(context.Background(), s.Export().ID)
	must(t, e)
	if !reflect.DeepEqual(got.Export(), s.Export()) {
		t.Fatalf("lost draft: %+v", got.Export())
	}
	list, e := store.List(context.Background())
	must(t, e)
	if len(list) != 1 || list[0].ID != s.Export().ID || list[0].Workspace != "/workspace" || list[0].Model != "test/model" || list[0].Status != domain.StatusInterrupted {
		t.Fatalf("bad list: %+v", list)
	}
	for _, p := range []string{dir, filepath.Join(dir, string(s.Export().ID)+".json"), filepath.Join(dir, string(s.Export().ID)+".lock")} {
		info, e := os.Stat(p)
		must(t, e)
		want := os.FileMode(0600)
		if info.IsDir() {
			want = 0700
		}
		if info.Mode().Perm() != want {
			t.Fatalf("permissions %s: %o", p, info.Mode().Perm())
		}
	}
	raw, e := os.ReadFile(filepath.Join(dir, string(s.Export().ID)+".json"))
	must(t, e)
	if strings.Contains(string(raw), "secret-not-for-disk") || strings.Contains(strings.ToLower(string(raw)), "api_key") {
		t.Fatal("credential persisted")
	}
}
func TestSessionStorePendingAndUncertainRecovery(t *testing.T) {
	store, _ := openStore(t)
	s := fixture(t)
	object, e := root.NewJSONObject([]byte(`{"type":"object"}`))
	must(t, e)
	identity, e := domain.NewLocalToolIdentity("read")
	must(t, e)
	must(t, s.BeginTurn("read", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "read", Parameters: object}, Identity: identity}}))
	must(t, s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "one", Name: "read", Arguments: object}, {ID: "two", Name: "read", Arguments: object}}}}))
	for _, checkpoint := range []bool{false, true} {
		if checkpoint {
			must(t, s.RecordToolOutcome("one", domain.DecisionApprove, domain.ToolOutcome{Content: "outcome unknown", IsError: true, Uncertain: true}))
			must(t, s.PauseTurn())
		}
		must(t, store.Save(context.Background(), s))
		got, e := store.Load(context.Background(), s.Export().ID)
		must(t, e)
		want := s.Export()
		want.Status = domain.StatusInterrupted
		if !reflect.DeepEqual(got.Export(), want) {
			t.Fatalf("changed checkpoint: %+v", got.Export())
		}
		if got.RecordToolOutcome("two", domain.DecisionApprove, domain.ToolOutcome{Content: "executed"}) == nil {
			t.Fatal("loaded approval active")
		}
	}
}
func TestSessionStoreRejectsVersionMalformedAndTraversal(t *testing.T) {
	store, dir := openStore(t)
	s := fixture(t)
	must(t, store.Save(context.Background(), s))
	path := filepath.Join(dir, string(s.Export().ID)+".json")
	raw, e := os.ReadFile(path)
	must(t, e)
	for _, change := range []func(map[string]any){func(m map[string]any) { m["version"] = 999 }, func(m map[string]any) { m["unknown"] = true }, func(m map[string]any) { m["id"] = strings.Repeat("a", 32) }} {
		var m map[string]any
		must(t, json.Unmarshal(raw, &m))
		change(m)
		bad, e := json.Marshal(m)
		must(t, e)
		must(t, os.WriteFile(path, bad, 0600))
		if _, e = store.Load(context.Background(), s.Export().ID); e == nil {
			t.Fatal("accepted invalid snapshot")
		}
	}
	for _, id := range []domain.SessionID{"../escape", "/tmp/escape", ""} {
		if _, e = store.Load(context.Background(), id); e == nil {
			t.Fatal("accepted unsafe ID")
		}
	}
}
func TestSessionStoreAtomicReplacementFailure(t *testing.T) {
	store, _ := openStore(t)
	s := fixture(t)
	must(t, store.Save(context.Background(), s))
	must(t, s.BeginTurn("new", nil))
	store.replace = func(string, string) error { return errors.New("simulated disk failure") }
	if e := store.Save(context.Background(), s); e == nil {
		t.Fatal("write failure hidden")
	}
	got, e := store.Load(context.Background(), s.Export().ID)
	must(t, e)
	if got.Status() != domain.StatusIdle || len(got.Messages()) != 0 {
		t.Fatal("old snapshot replaced")
	}
}
func TestSessionStoreLockAndClose(t *testing.T) {
	store, dir := openStore(t)
	s := fixture(t)
	must(t, store.Save(context.Background(), s))
	second, e := New(dir)
	must(t, e)
	defer second.Close()
	if e = second.Save(context.Background(), s); e == nil {
		t.Fatal("second writer accepted")
	}
	other, e := domain.NewSession("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "/workspace", "test/model")
	must(t, e)
	must(t, second.Save(context.Background(), other))
	must(t, store.Close())
	must(t, second.Save(context.Background(), s))
	if e = store.Save(context.Background(), s); e == nil {
		t.Fatal("closed store writable")
	}
}
func TestSessionStoreDefaultDirectory(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	store, e := New("")
	must(t, e)
	defer store.Close()
	must(t, store.Save(context.Background(), fixture(t)))
	if _, e = os.Stat(filepath.Join(state, "axlr", "sessions", string(fixture(t).Export().ID)+".json")); e != nil {
		t.Fatal(e)
	}
}
