package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestSessionModeIsKeptBesideTheSnapshot(t *testing.T) {
	store, dir := openStore(t)
	const id = "0123456789abcdef0123456789abcdef"
	s, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(domain.ModeWriter); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil || strings.Contains(string(snapshot), "writer") {
		t.Fatalf("mode leaked into the snapshot: %s %v", snapshot, err)
	}
	loaded, err := store.Load(context.Background(), id)
	if err != nil || loaded.Mode() != domain.ModeWriter {
		t.Fatalf("%v %q", err, loaded.Mode())
	}
	if err := loaded.SetMode(domain.ModeNormal); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, id+".mode")); !os.IsNotExist(err) {
		t.Fatal("normal mode left a sidecar behind")
	}
}

func TestUnreadableModeSidecarLoadsAsNormal(t *testing.T) {
	store, dir := openStore(t)
	const id = "fedcba9876543210fedcba9876543210"
	s, _ := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err := store.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".mode"), []byte(`{"version":1,"mode":"loud"}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), id)
	if err != nil || loaded.Mode() != domain.ModeNormal {
		t.Fatalf("%v %q", err, loaded.Mode())
	}
}

func TestCeremonyRunIsKeptBesideTheSnapshot(t *testing.T) {
	store, dir := openStore(t)
	const id = "00112233445566778899aabbccddeeff"
	s, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(domain.ModeDebug); err != nil {
		t.Fatal(err)
	}
	run := domain.CeremonyRun{Definition: "axlr_debug", Version: "2.0", Instance: "axlr-x", Step: "repair", Iteration: 2, Fence: "fence-9", About: "ws:" + id, Check: domain.CheckCommand{Program: "python3", Args: []string{"-m", "unittest"}}, BudgetBase: 7, Reminded: true}
	if err := s.SetCeremony(run); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Ceremony()
	if !ok || got.Step != "repair" || got.Iteration != 2 || got.Fence != "fence-9" || !got.Check.Equal(run.Check) || got.About != run.About || got.BudgetBase != 7 || !got.Reminded {
		t.Fatalf("run not restored: %+v", got)
	}
	loaded.FinishCeremony()
	if err := store.Save(context.Background(), loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, id+".ceremony")); !os.IsNotExist(err) {
		t.Fatal("finished ceremony left a sidecar behind")
	}
}

func TestACeremonySidecarWithANewerFieldStillLoads(t *testing.T) {
	store, dir := openStore(t)
	const id = "0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f"
	s, _ := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err := store.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	record := `{"version":1,"definition":"axlr_debug","release":"2.0","instance":"axlr-y","step":"diagnose","iteration":1,"fence":"f","from_a_newer_console":true}`
	if err := os.WriteFile(filepath.Join(dir, id+".ceremony"), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if run, ok := loaded.Ceremony(); !ok || run.Instance != "axlr-y" {
		t.Fatal("an unknown field made the live ceremony disappear")
	}
}
