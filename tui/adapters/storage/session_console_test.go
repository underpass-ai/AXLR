package storage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 10 October 2026 /stop-ceremony was not told to the model; the note the
// next prompt carries must survive a console restart in between.
func TestStoppedCeremonyIsKeptBesideTheSnapshot(t *testing.T) {
	store, dir := openStore(t)
	const id = "00112233445566778899aabbccddeeff"
	s, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	must(t, s.BeginTurn("plan it", nil))
	s.NoteStoppedCeremony("axlr_plan")
	must(t, store.Save(context.Background(), s))
	snapshot, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil || strings.Contains(string(snapshot), "axlr_plan") {
		t.Fatalf("the note leaked into the snapshot, which older consoles reject: %s %v", snapshot, err)
	}
	loaded, err := store.Load(context.Background(), id)
	if err != nil || loaded.Export().StoppedCeremony != "axlr_plan" {
		t.Fatalf("%v %q", err, loaded.Export().StoppedCeremony)
	}
	if loaded.TakeStoppedCeremony() != "axlr_plan" {
		t.Fatal("the loaded note is not taken")
	}
	must(t, store.Save(context.Background(), loaded))
	if _, err := os.Stat(filepath.Join(dir, id+".console")); !os.IsNotExist(err) {
		t.Fatal("a taken note left a sidecar behind")
	}
}

func TestSessionWithoutConsoleSidecarLoads(t *testing.T) {
	store, dir := openStore(t)
	const id = "ffeeddccbbaa99887766554433221100"
	s, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	must(t, s.BeginTurn("hello", nil))
	must(t, store.Save(context.Background(), s))
	// A session saved by an older console has no <id>.console.
	if _, err := os.Stat(filepath.Join(dir, id+".console")); !os.IsNotExist(err) {
		t.Fatal("a session with nothing to note wrote a sidecar")
	}
	loaded, err := store.Load(context.Background(), id)
	if err != nil || loaded.Export().StoppedCeremony != "" || len(loaded.ConsoleLines()) != 0 {
		t.Fatalf("%v %+v", err, loaded.Export())
	}
	// A newer console's field does not hide the ones this one knows; a
	// broken file loses the notes, never the session.
	must(t, os.WriteFile(filepath.Join(dir, id+".console"), []byte(`{"version":1,"stopped_ceremony":"axlr_debug","later":true}`), 0600))
	if loaded, err = store.Load(context.Background(), id); err != nil || loaded.Export().StoppedCeremony != "axlr_debug" {
		t.Fatalf("%v %q", err, loaded.Export().StoppedCeremony)
	}
	must(t, os.WriteFile(filepath.Join(dir, id+".console"), []byte(`{"version":1,`), 0600))
	if loaded, err = store.Load(context.Background(), id); err != nil || loaded.Export().StoppedCeremony != "" {
		t.Fatalf("%v %q", err, loaded.Export().StoppedCeremony)
	}
}

// plannedSession is a planning session whose plan finished: its transcript
// ends with the planner's answer and a console line with the outcome.
func plannedSession(t *testing.T, id domain.SessionID) domain.Session {
	t.Helper()
	s, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	must(t, err)
	must(t, s.BeginTurn("add buildinfo.Short()", nil))
	must(t, s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "It's waiting for your approval on the plan card."}}))
	added, err := s.AddPlanOutcome(domain.PlanOutcome{PlanID: "add-a-func-dce953", Status: domain.PlanDone, Tasks: 2, Done: 1, Skipped: 1,
		Syncs: []domain.SyncVerdict{{Wave: 1, Verdict: "green"}}, Changed: []string{"buildinfo/version.go"}, CostUSD: 0.008, CostKnown: true})
	if err != nil || !added {
		t.Fatalf("%v %v", added, err)
	}
	return s
}

// On 10 October 2026 a finished plan left no trace in its planning session;
// the outcome's console line must survive the console closing.
func TestConsoleLinesAreKeptBesideTheSnapshot(t *testing.T) {
	store, dir := openStore(t)
	const id = "0f1e2d3c4b5a69788796a5b4c3d2e1f0"
	s := plannedSession(t, id)
	must(t, store.Save(context.Background(), s))
	snapshot, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil || strings.Contains(string(snapshot), "add-a-func-dce953") {
		t.Fatalf("the line leaked into the snapshot, which older consoles reject: %s %v", snapshot, err)
	}
	loaded, err := store.Load(context.Background(), id)
	if err != nil || !reflect.DeepEqual(loaded.ConsoleLines(), s.ConsoleLines()) {
		t.Fatalf("%v\n got %+v\nwant %+v", err, loaded.ConsoleLines(), s.ConsoleLines())
	}
	// Lines that no longer fit the transcript are dropped, not the session.
	must(t, os.WriteFile(filepath.Join(dir, id+".console"), []byte(`{"version":1,"lines":[{"after_message":9,"at":"2026-10-10T10:00:00Z","plan":{"id":"add-a-func-dce953","status":"done","tasks":1,"done":1}}]}`), 0600))
	loaded, err = store.Load(context.Background(), id)
	if err != nil || len(loaded.ConsoleLines()) != 0 || len(loaded.Messages()) != 2 {
		t.Fatalf("%v %+v", err, loaded.ConsoleLines())
	}
}

// A session whose only content is a console line is not discarded as empty
// when the console closes.
func TestSessionWithConsoleLinesIsNotEmpty(t *testing.T) {
	store, _ := openStore(t)
	const id = "a0a1a2a3a4a5a6a7a8a9aaabacadaeaf"
	s, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	must(t, err)
	if _, err := s.AddPlanOutcome(domain.PlanOutcome{PlanID: "add-a-func-dce953", Status: domain.PlanBlocked}); err != nil {
		t.Fatal(err)
	}
	must(t, store.Save(context.Background(), s))
	if store.isEmpty(id) {
		t.Fatal("a planning session reads as empty")
	}
}

// A restore's console line names the steps it went back over and survives
// the console closing, beside the plan outcomes.
func TestRestoreLinesAreKeptBesideTheSnapshot(t *testing.T) {
	store, _ := openStore(t)
	const id = "0f1e2d3c4b5a69788796a5b4c3d2e1f0"
	s := plannedSession(t, id)
	must(t, s.AddRestore(domain.FileRestore{Undo: true, Files: []domain.RestoredFile{{Path: "a.go", From: "call-2", To: "call-1"}}}))
	must(t, store.Save(context.Background(), s))
	loaded, err := store.Load(context.Background(), id)
	lines := loaded.ConsoleLines()
	if err != nil || len(lines) != 2 || !reflect.DeepEqual(lines, s.ConsoleLines()) || lines[1].Restore == nil || !lines[1].Restore.Undo {
		t.Fatalf("%v\n got %+v\nwant %+v", err, lines, s.ConsoleLines())
	}
}
