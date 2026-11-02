package storage

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The ledger lives in a private file beside the snapshot, survives a new
// store (a restart) and is the same ledger it was.
func TestSessionUsageIsKeptBesideTheSnapshot(t *testing.T) {
	store, dir := openStore(t)
	ctx := context.Background()
	const id = domain.SessionID("0123456789abcdef0123456789abcdef")
	if empty, err := store.LoadUsage(ctx, id); err != nil || !reflect.DeepEqual(empty, domain.UsageLedger{}) {
		t.Fatalf("missing ledger: %+v %v", empty, err)
	}
	saved, err := store.UpdateUsage(ctx, id, func(l *domain.UsageLedger) {
		l.Record(domain.UsageSample{Provider: "Relace", PromptTokens: 7448, CachedTokens: 1, CacheWriteTokens: 2, CompletionTokens: 2567, ReasoningTokens: 2590, Cost: 0.00158142, CostKnown: true, FirstByte: 1500 * time.Millisecond, Total: 9 * time.Second}, 1)
		l.Record(domain.UsageSample{Provider: "local/qwen", PromptTokens: 10, Total: time.Second}, 1)
		l.Record(domain.UsageSample{Provider: "Anthropic", PromptTokens: 4000, CompletionTokens: 300, Total: 10 * time.Second, Unmetered: true, GenerationID: "gen-1", Ended: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)}, 1)
		l.Missed("gen-1", time.Date(2026, 11, 2, 10, 0, 5, 0, time.UTC))
		l.Raise(1)
		l.Warned = true
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, string(id)+".usage.json"))
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("ledger file: %v %v", info, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	loaded, err := reopened.LoadUsage(ctx, id)
	if err != nil || !reflect.DeepEqual(loaded, saved) || loaded.Providers["Relace"].Cost != 0.00158142 || loaded.FirstByte.Max != 1500*time.Millisecond || loaded.Raised != 1 || loaded.Unmetered != 1 || len(loaded.Pending) != 1 || loaded.Pending[0].Lookups != 1 {
		t.Fatalf("loaded %+v\nsaved %+v\n%v", loaded, saved, err)
	}
	if _, err := reopened.LoadUsage(ctx, "../escape"); err == nil {
		t.Fatal("an invalid session id was read")
	}
}

// A damaged ledger, or one from another version, starts again instead of
// blocking the session's requests.
func TestADamagedSessionUsageStartsAgain(t *testing.T) {
	store, dir := openStore(t)
	ctx := context.Background()
	const id = domain.SessionID("0123456789abcdef0123456789abcdef")
	for _, content := range []string{"{not json", `{"version":2,"requests":5}`} {
		if err := os.WriteFile(filepath.Join(dir, string(id)+".usage.json"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if ledger, err := store.LoadUsage(ctx, id); err != nil || ledger.Requests != 0 {
			t.Fatalf("%s: %+v %v", content, ledger, err)
		}
		ledger, err := store.UpdateUsage(ctx, id, func(l *domain.UsageLedger) { l.Record(domain.UsageSample{Cost: 0.5, CostKnown: true}, 0) })
		if err != nil || ledger.Requests != 1 || ledger.Cost != 0.5 {
			t.Fatalf("%s: %+v %v", content, ledger, err)
		}
	}
}

// A ledger an older console wrote, without pending generations, loads
// with the numbers it had: the file's version did not change.
func TestAnOlderSessionUsageStillLoads(t *testing.T) {
	store, dir := openStore(t)
	const id = domain.SessionID("0123456789abcdef0123456789abcdef")
	old := `{"version":1,"requests":3,"prompt_tokens":5000,"cached_tokens":0,"cache_write_tokens":0,"completion_tokens":400,"reasoning_tokens":0,"cost_usd":0.25,"cost_requests":2,` +
		`"providers":{"Anthropic":{"requests":3,"prompt_tokens":5000,"completion_tokens":400,"cost_usd":0.25,"cost_requests":2}},"first_byte":{"count":2,"sum_ms":900,"max_ms":600},"total":{"count":3,"sum_ms":9000,"max_ms":4000},"unmetered":1}`
	if err := os.WriteFile(filepath.Join(dir, string(id)+".usage.json"), []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	ledger, err := store.LoadUsage(context.Background(), id)
	if err != nil || ledger.Requests != 3 || ledger.Cost != 0.25 || ledger.Unmetered != 1 || ledger.Pending != nil || ledger.Providers["Anthropic"].CostRequests != 2 {
		t.Fatalf("old ledger %+v %v", ledger, err)
	}
}
