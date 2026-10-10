package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
