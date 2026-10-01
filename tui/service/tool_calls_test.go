package service

import (
	"path/filepath"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestCallRecoveryNeverReexecutesApprovedEffect(t *testing.T) {
	store, err := newCallStore(filepath.Join(t.TempDir(), "calls"))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := domain.NewLocalToolIdentity("write")
	if err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	call := toolCall{ID: id, Owner: "alice", Tool: "local_write", Identity: identity, Args: []byte(`{"path":"x","content":"a","mode":"create"}`), Status: "pending_approval", Decision: "approve", Revision: 1}
	if err := store.Save(call); err != nil {
		t.Fatal(err)
	}
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "uncertain" || loaded.Revision != 2 || loaded.Result == nil || !loaded.Result.Uncertain {
		t.Fatalf("unsafe recovery: %+v", loaded)
	}
	if err := store.Recover(); err != nil {
		t.Fatal(err)
	}
	again, err := store.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if again.Revision != 2 {
		t.Fatalf("repeated recovery changed call: %+v", again)
	}
}
