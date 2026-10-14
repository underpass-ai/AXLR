package storage

import (
	"context"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// A session another console has open is held elsewhere; one this console
// holds, or one nobody holds, is not.
func TestHeldElsewhereTellsAnotherConsolesSession(t *testing.T) {
	dir := t.TempDir()
	mine, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mine.Close()
	other, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), "model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("hola", nil); err != nil {
		t.Fatal(err)
	}
	if mine.HeldElsewhere(s.ID()) {
		t.Fatal("a session never saved is held")
	}
	if err := other.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if !mine.HeldElsewhere(s.ID()) {
		t.Fatal("another console's session is not held elsewhere")
	}
	if other.HeldElsewhere(s.ID()) {
		t.Fatal("the console's own session reads as held elsewhere")
	}
	// The probe takes nothing: the holder still writes, and once it lets
	// go the session is free.
	if err := other.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if err := other.Release(s.ID()); err != nil {
		t.Fatal(err)
	}
	if mine.HeldElsewhere(s.ID()) {
		t.Fatal("a released session is still held")
	}
}
