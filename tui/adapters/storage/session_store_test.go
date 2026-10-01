package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

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
	s, e := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), "test/model")
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
	if len(list) != 1 || list[0].ID != s.Export().ID || list[0].Workspace != s.Export().Workspace || list[0].Model != "test/model" || list[0].Status != domain.StatusInterrupted {
		t.Fatalf("bad list: %+v", list)
	}
	if list[0].Title != "hello" || list[0].MessageCount != len(s.Export().Messages) || list[0].MessageCount == 0 || time.Since(list[0].UpdatedAt) > time.Minute {
		t.Fatalf("list lacks title, message count or save time: %+v", list[0])
	}
	for _, p := range []string{dir, filepath.Join(dir, string(s.Export().ID)+".json"), filepath.Join(dir, string(s.Export().ID)+".lock")} {
		info, e := os.Stat(p)
		must(t, e)
		want := os.FileMode(0600)
		if info.IsDir() {
			want = 0700
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != want {
			t.Fatalf("permissions %s: %o", p, info.Mode().Perm())
		}
	}
	raw, e := os.ReadFile(filepath.Join(dir, string(s.Export().ID)+".json"))
	must(t, e)
	if strings.Contains(string(raw), "secret-not-for-disk") || strings.Contains(strings.ToLower(string(raw)), "api_key") {
		t.Fatal("credential persisted")
	}
}

func TestSessionStoreRestoresChangedModelFromVersionOneSnapshot(t *testing.T) {
	store, dir := openStore(t)
	s := fixture(t)
	must(t, s.BeginTurn("before", nil))
	must(t, s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "answer"}}))
	before := s.Messages()
	must(t, s.ChangeModel("next/model"))
	must(t, store.Save(context.Background(), s))
	raw, err := os.ReadFile(filepath.Join(dir, string(s.Export().ID)+".json"))
	must(t, err)
	var record map[string]any
	must(t, json.Unmarshal(raw, &record))
	if record["version"] != float64(2) {
		t.Fatalf("snapshot version changed: %v", record["version"])
	}
	got, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	if got.Export().Model != "next/model" || !reflect.DeepEqual(got.Messages(), before) {
		t.Fatalf("restored model or transcript changed: %+v", got.Export())
	}
}
func TestSessionStoreReadsVersionOneAndPreservesServiceMetadata(t *testing.T) {
	store, dir := openStore(t)
	s := fixture(t)
	must(t, store.Save(context.Background(), s))
	path := filepath.Join(dir, string(s.Export().ID)+".json")
	raw, err := os.ReadFile(path)
	must(t, err)
	var record map[string]any
	must(t, json.Unmarshal(raw, &record))
	record["version"] = 1
	legacy, err := json.Marshal(record)
	must(t, err)
	must(t, os.WriteFile(path, legacy, 0600))
	loaded, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	if loaded.Export().Owner != "" || loaded.Export().Revision != 0 {
		t.Fatalf("legacy metadata not empty: %+v", loaded.Export())
	}
	loaded.SetServiceMetadata("alice", 7, "operation-1")
	must(t, store.Save(context.Background(), loaded))
	got, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	if got.Export().Owner != "alice" || got.Export().Revision != 7 || got.Export().OperationID != "operation-1" {
		t.Fatalf("service metadata lost: %+v", got.Export())
	}
}
func TestSessionStorePreservesInterruptedAnswersInConversationOrder(t *testing.T) {
	store, _ := openStore(t)
	s := fixture(t)
	must(t, s.BeginTurn("first", nil))
	must(t, s.InterruptDraft("first partial"))
	must(t, s.BeginTurn("second", nil))
	must(t, s.InterruptDraft("second partial"))
	must(t, store.Save(context.Background(), s))
	got, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	if !reflect.DeepEqual(got.Export(), s.Export()) {
		t.Fatalf("archived answer changed after reload: %+v", got.Export())
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
	other, e := domain.NewSession("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", domain.Workspace(t.TempDir()), "test/model")
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

func TestSessionStoreCloseDiscardsOnlyEmptySessions(t *testing.T) {
	store, dir := openStore(t)
	empty := fixture(t)
	used, err := domain.NewSession("1123456789abcdef0123456789abcdef", empty.Export().Workspace, "test/model")
	must(t, err)
	must(t, used.BeginTurn("keep me", nil))
	must(t, store.Save(context.Background(), empty))
	must(t, store.Save(context.Background(), used))
	must(t, store.Close())
	for id, kept := range map[domain.SessionID]bool{empty.Export().ID: false, used.Export().ID: true} {
		_, err := os.Stat(filepath.Join(dir, string(id)+".json"))
		if kept != (err == nil) {
			t.Fatalf("%s kept=%v, stat err=%v", id, kept, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, string(empty.Export().ID)+".lock")); !os.IsNotExist(err) {
		t.Fatalf("empty session's lock left behind: %v", err)
	}
}

func TestSessionStoreKeepsMessageTimesBesideTheSnapshot(t *testing.T) {
	store, dir := openStore(t)
	s := fixture(t)
	must(t, s.BeginTurn("hello", nil))
	must(t, store.Save(context.Background(), s))
	raw, err := os.ReadFile(filepath.Join(dir, string(s.Export().ID)+".json"))
	must(t, err)
	if strings.Contains(string(raw), "time") {
		t.Fatal("snapshot format changed")
	}
	got, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	if !reflect.DeepEqual(got.Export().MessageTimes, s.Export().MessageTimes) || len(got.Export().MessageTimes) != 1 {
		t.Fatalf("times = %v, want %v", got.Export().MessageTimes, s.Export().MessageTimes)
	}
	must(t, os.Remove(filepath.Join(dir, string(s.Export().ID)+".times")))
	got, err = store.Load(context.Background(), s.Export().ID)
	must(t, err)
	if len(got.Export().MessageTimes) != 0 || len(got.Export().Messages) != 1 {
		t.Fatal("a session without times did not load")
	}
}
