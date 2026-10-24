package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// A session's policy is saved beside its snapshot and comes back with it;
// a policy file the store cannot read refuses the session rather than
// loading it without its policy.
func TestSessionStoreKeepsThePolicy(t *testing.T) {
	store, dir := openStore(t)
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	session, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := domain.NewExecutionPolicy(domain.PolicyObserve, []domain.PolicyRule{{Name: "local_read", Arguments: map[string]string{"path": "logs/**"}}}, []domain.PolicyRule{{Name: "local_exec"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.SetPolicy(policy); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, string(id)+".policy")); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	kept, has := loaded.Policy()
	if !has || kept.Level() != domain.PolicyObserve || len(kept.Allow()) != 1 || kept.Allow()[0].Arguments["path"] != "logs/**" || len(kept.Deny()) != 1 {
		t.Fatalf("%+v", kept)
	}
	loaded.ClearPolicy()
	if err := store.Save(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, string(id)+".policy")); !os.IsNotExist(err) {
		t.Fatalf("the policy file stays after clearing: %v", err)
	}
	if again, err := store.Load(ctx, id); err != nil {
		t.Fatal(err)
	} else if _, has := again.Policy(); has {
		t.Fatal("a cleared policy came back")
	}
	if err := os.WriteFile(filepath.Join(dir, string(id)+".policy"), []byte(`{"version":1,"policy":{"level":"autonomous","tools":{}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, id); err == nil {
		t.Fatal("a session with an unreadable policy loaded")
	}
}
