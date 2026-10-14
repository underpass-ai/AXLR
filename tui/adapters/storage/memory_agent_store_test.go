package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallIDIsDrawnOnceAndKept(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "axlr")
	first, err := InstallID(dir)
	if err != nil || !isHexID(first) {
		t.Fatalf("%q %v", first, err)
	}
	second, err := InstallID(dir)
	if err != nil || second != first {
		t.Fatalf("%q %q %v", first, second, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "install-id"), []byte("garbage\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	third, err := InstallID(dir)
	if err != nil || third == first || !isHexID(third) {
		t.Fatalf("an unreadable id is drawn again: %q %v", third, err)
	}
}

func TestKMPAgentStoreKeepsOneAgentPerStore(t *testing.T) {
	agents := NewMemoryAgentStore(filepath.Join(t.TempDir(), "axlr", "kmp-agents.json"))
	ctx := context.Background()
	if _, ok := agents.Agent(ctx, "/s/.kernel"); ok {
		t.Fatal("empty store has no agent")
	}
	if err := agents.Remember(ctx, "/s/.kernel", "agent_1", "axlr:install"); err != nil {
		t.Fatal(err)
	}
	if err := agents.Remember(ctx, "/t/.kernel/", "agent_2", "axlr:install"); err != nil {
		t.Fatal(err)
	}
	if id, ok := agents.Agent(ctx, "/s/.kernel/"); !ok || id != "agent_1" {
		t.Fatalf("%q %v", id, ok)
	}
	if id, ok := agents.Agent(ctx, "/t/.kernel"); !ok || id != "agent_2" {
		t.Fatalf("%q %v", id, ok)
	}
	if err := agents.Remember(ctx, "", "agent_3", "k"); err == nil {
		t.Fatal("a store path is required")
	}
	reopened := NewMemoryAgentStore(agents.path)
	if id, ok := reopened.Agent(ctx, "/s/.kernel"); !ok || id != "agent_1" {
		t.Fatalf("not persisted: %q %v", id, ok)
	}
}
