package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestSessionStoreRevisionAndLegacyOwner(t *testing.T) {
	base, err := storage.New(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	wrapped := &sessionStore{next: base}
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	s.SetServiceMetadata("alice", 0, "")
	for revision := uint64(1); revision <= 2; revision++ {
		if err := wrapped.Save(context.Background(), s); err != nil {
			t.Fatal(err)
		}
		got, err := wrapped.Load(context.Background(), s.Export().ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Export().Owner != "alice" || got.Export().Revision != revision {
			t.Fatalf("state: %+v", got.Export())
		}
	}
	s.SetServiceMetadata("bob", 0, "")
	if err := wrapped.Save(context.Background(), s); err == nil {
		t.Fatal("owner change accepted")
	}
}

func TestEventJournalReplayRecovery(t *testing.T) {
	dir := t.TempDir()
	journal, err := newEventStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	first, err := journal.Append(id, "op", "turn.started", map[string]any{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	if first.Sequence != 1 {
		t.Fatal(first)
	}
	if err := os.WriteFile(journal.path(id), append(mustRead(t, journal.path(id)), []byte("bro")...), 0600); err != nil {
		t.Fatal(err)
	}
	got, _, err := journal.Read(id, 0)
	if err != nil || len(got) != 1 {
		t.Fatalf("replay: %v %+v", err, got)
	}
	second, err := journal.Append(id, "op", "turn.completed", nil)
	if err != nil || second.Sequence != 2 {
		t.Fatalf("append: %v %+v", err, second)
	}
	got, _, err = journal.Read(id, 1)
	if err != nil || len(got) != 1 || got[0].Type != "turn.completed" {
		t.Fatalf("cursor: %v %+v", err, got)
	}
}

func TestEventJournalRejectsInteriorCorruptionWithoutTruncating(t *testing.T) {
	journal, err := newEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	for _, kind := range []string{"turn.started", "text.delta", "turn.completed"} {
		if _, err := journal.Append(id, "op", kind, nil); err != nil {
			t.Fatal(err)
		}
	}
	path := journal.path(id)
	data := mustRead(t, path)
	firstLength := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])
	corrupt := 8 + firstLength + 8 + 10
	data[corrupt] ^= 0x01
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := journal.Read(id, 0); err == nil {
		t.Fatal("interior corruption was accepted")
	}
	if got := mustRead(t, path); !bytes.Equal(got, data) {
		t.Fatal("corrupt journal was silently truncated")
	}
}

func TestIdempotencyClaimPersistsAndConflicts(t *testing.T) {
	dir := t.TempDir()
	store, err := newIdempotencyStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	resource, duplicate, err := store.Claim("alice", "0123456789abcdef", "POST", "/v1/turns", []byte(`{"prompt":"one"}`), "op1")
	if err != nil || duplicate || resource != "op1" {
		t.Fatalf("first: %q %t %v", resource, duplicate, err)
	}
	store, err = newIdempotencyStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	resource, duplicate, err = store.Claim("alice", "0123456789abcdef", "POST", "/v1/turns", []byte(`{"prompt":"one"}`), "op2")
	if err != nil || !duplicate || resource != "op1" {
		t.Fatalf("repeat: %q %t %v", resource, duplicate, err)
	}
	_, _, err = store.Claim("alice", "0123456789abcdef", "POST", "/v1/turns", []byte(`{"prompt":"two"}`), "op3")
	if !errors.Is(err, errIdempotencyConflict) {
		t.Fatalf("conflict: %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
