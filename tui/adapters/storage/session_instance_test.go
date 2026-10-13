package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The session's ceremony instance lives in <id>.instance beside the snapshot,
// survives a reload and goes with a discarded or deleted session.
func TestSessionInstanceSidecarRoundTripsAndIsDiscarded(t *testing.T) {
	store, dir := openStore(t)
	s := fixture(t)
	must(t, s.BeginTurn("hola", nil))
	must(t, s.SetInstance(domain.SessionInstance{Instance: "axlr-session-" + string(s.Export().ID), Turns: 3, Fence: "fence-1", TurnIndex: 0, ClaimRequests: 2, ClaimCost: 0.01, LastRecord: `{"turn":3}`}))
	must(t, store.Save(context.Background(), s))
	path := filepath.Join(dir, string(s.Export().ID)+".instance")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	got, has := loaded.Instance()
	if !has || got.Instance != "axlr-session-"+string(s.Export().ID) || got.Turns != 3 || got.Fence != "fence-1" || got.ClaimRequests != 2 || got.LastRecord != `{"turn":3}` {
		t.Fatalf("loaded %+v", got)
	}
	// An unreadable sidecar loses only the pointer.
	must(t, os.WriteFile(path, []byte("{not json"), 0o600))
	loaded, err = store.Load(context.Background(), s.Export().ID)
	must(t, err)
	if _, has := loaded.Instance(); has {
		t.Fatal("an unreadable sidecar must read as none")
	}
	must(t, store.Save(context.Background(), s))
	must(t, store.Delete(context.Background(), s.Export().ID))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("sidecar kept after delete: %v", err)
	}
	empty, e := domain.NewSession("3123456789abcdef0123456789abcdef", s.Export().Workspace, "test/model")
	must(t, e)
	must(t, empty.SetInstance(domain.SessionInstance{Unavailable: "not prepared"}))
	must(t, store.Save(context.Background(), empty))
	must(t, store.Close())
	if _, err := os.Stat(filepath.Join(dir, string(empty.Export().ID)+".instance")); !os.IsNotExist(err) {
		t.Fatalf("sidecar kept after discard: %v", err)
	}
}

// A sidecar written before the memory ids were typed (keys kmp_agent and
// kmp_context, a predecessor as text) loads into the same instance, and
// the console writes the same keys back.
func TestSessionInstanceSidecarKeepsTheKeysOnDisk(t *testing.T) {
	store, dir := openStore(t)
	s := fixture(t)
	must(t, s.BeginTurn("hola", nil))
	must(t, store.Save(context.Background(), s))
	id := string(s.Export().ID)
	path := filepath.Join(dir, id+".instance")
	written := `{"version":1,"instance":"axlr-session-` + id + `","turns":2,"turn_index":0,"kmp_agent":"agent_1","kmp_context":"context_1","predecessor":"fedcba9876543210fedcba9876543210"}`
	must(t, os.WriteFile(path, []byte(written), 0o600))
	loaded, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	got, has := loaded.Instance()
	if !has || got.MemoryAgent != "agent_1" || got.MemoryContext != "context_1" || got.Predecessor != "fedcba9876543210fedcba9876543210" {
		t.Fatalf("loaded %+v", got)
	}
	must(t, store.Save(context.Background(), loaded))
	data, err := os.ReadFile(path)
	must(t, err)
	for _, key := range []string{`"kmp_agent":"agent_1"`, `"kmp_context":"context_1"`, `"predecessor":"fedcba9876543210fedcba9876543210"`} {
		if !strings.Contains(strings.ReplaceAll(string(data), " ", ""), key) {
			t.Fatalf("written %s lacks %s", data, key)
		}
	}
}
