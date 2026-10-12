package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 12 October 2026 Ctrl+O could only archive a session. Deleting one
// removes its snapshot and every file beside it, and refuses a session
// another console has open.
func TestDeleteRemovesASessionAndRefusesOneOpenElsewhere(t *testing.T) {
	store, dir := openStore(t)
	s := fixture(t)
	must(t, s.BeginTurn("hello", nil))
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "hi"}})
	must(t, store.Save(context.Background(), s))
	if _, err := store.UpdateUsage(context.Background(), s.Export().ID, func(l *domain.UsageLedger) { l.Requests++ }); err != nil {
		t.Fatal(err)
	}
	id := s.Export().ID

	other, err := New(dir)
	must(t, err)
	defer other.Close()
	if err := other.Delete(context.Background(), id); err == nil {
		t.Fatal("another console deleted a session this one has open")
	}

	must(t, store.Delete(context.Background(), id))
	entries, err := os.ReadDir(dir)
	must(t, err)
	for _, entry := range entries {
		// Windows cannot remove a file it holds open: the lock may stay.
		if runtime.GOOS != "windows" || filepath.Ext(entry.Name()) != ".lock" {
			t.Errorf("left behind: %s", entry.Name())
		}
	}
	if err := store.Delete(context.Background(), id); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleting it again: %v", err)
	}
	// A session no console holds is deleted by any of them.
	again := fixture(t)
	must(t, again.BeginTurn("again", nil))
	must(t, store.Save(context.Background(), again))
	must(t, store.Close())
	must(t, other.Delete(context.Background(), again.Export().ID))
	if _, err := os.Stat(filepath.Join(dir, string(again.Export().ID)+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("snapshot still there: %v", err)
	}
	if err := other.Delete(context.Background(), "../escape"); err == nil {
		t.Fatal("an invalid id was accepted")
	}
}
