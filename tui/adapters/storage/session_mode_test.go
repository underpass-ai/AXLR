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
