package storage

import (
	"context"
	"encoding/json"
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

// The largest instance the domain allows is read back whole: at 64 KiB a
// full pending queue was written and read as no sidecar (14 October 2026).
func TestSessionInstanceSidecarRoundTripsAMaximalInstance(t *testing.T) {
	store, _ := openStore(t)
	s := fixture(t)
	must(t, s.BeginTurn("hola", nil))
	evidence := make([]string, 8)
	for i := range evidence {
		evidence[i] = strings.Repeat(`"q" <&> `, 125)
	}
	entry, err := json.Marshal(map[string]any{"About": "ws:x", "Key": "axlr:turn:x:1", "Summary": strings.Repeat(`"s" `, 100), "Evidence": evidence, "Labels": map[string][]string{"ws": {"/ws"}, "session": {"x"}}})
	must(t, err)
	instance := domain.SessionInstance{Instance: "axlr-session-" + string(s.Export().ID), Turns: 9, Fence: "fence-9", MemoryContext: "ctx_1", LastRecord: `{"answer":"` + strings.Repeat(`\"`, (domain.MaxSessionRecordBytes-20)/2) + `"}`}
	for i := 0; i < domain.MaxPendingEntries; i++ {
		instance.Pending = append(instance.Pending, string(entry))
	}
	for i := 0; i < 8; i++ {
		instance.RecalledMemories = append(instance.RecalledMemories, strings.Repeat(`"r" `, 75))
	}
	must(t, s.SetInstance(instance))
	must(t, store.Save(context.Background(), s))
	loaded, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	got, has := loaded.Instance()
	if !has || got.Fence != "fence-9" || got.MemoryContext != "ctx_1" || got.LastRecord != instance.LastRecord || len(got.Pending) != domain.MaxPendingEntries || got.Pending[0] != string(entry) || len(got.RecalledMemories) != 8 {
		t.Fatalf("loaded has=%v fence %q context %q pending %d", has, got.Fence, got.MemoryContext, len(got.Pending))
	}
}

// A sidecar over the bound is trimmed when written, the last record first,
// then the oldest entries, so what is written is read back.
func TestFitInstanceTrimsWhatTheReadWouldRefuse(t *testing.T) {
	big := strings.Repeat("x", maxSessionInstanceBytes/10)
	record := sessionInstance{Version: sessionInstanceVersion, Instance: "axlr-session-x", Fence: "f", LastRecord: "{}"}
	for i := 0; i < 12; i++ {
		record.Pending = append(record.Pending, big)
	}
	data, err := fitInstance(record)
	must(t, err)
	var fitted sessionInstance
	must(t, json.Unmarshal(data, &fitted))
	if len(data) > maxSessionInstanceBytes || fitted.LastRecord != "" || fitted.Fence != "f" || len(fitted.Pending) == 0 || len(fitted.Pending) >= 12 {
		t.Fatalf("%d bytes, record %q, pending %d", len(data), fitted.LastRecord, len(fitted.Pending))
	}
}
