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

// The journal is appended in a private file beside the snapshot and reads
// back the same entries after a new store (a restart); a line a crash cut
// is skipped, and an entry past the bound is refused.
func TestSessionReceiptIsJournaledBesideTheSnapshot(t *testing.T) {
	store, dir := openStore(t)
	ctx := context.Background()
	const id = domain.SessionID("0123456789abcdef0123456789abcdef")
	if entries, err := store.LoadReceipt(ctx, id); err != nil || entries != nil {
		t.Fatalf("missing journal: %+v %v", entries, err)
	}
	at := time.Date(2026, 10, 25, 9, 0, 0, 123456789, time.UTC)
	want := []domain.ReceiptEntry{
		{Kind: domain.ReceiptRequestStarted, At: at, Model: "anthropic/claude-haiku-5.5"},
		{Kind: domain.ReceiptRequestEnded, At: at.Add(time.Second), Status: domain.ReceiptRequestCompleted, Usage: domain.UsageSample{Provider: "Anthropic", PromptTokens: 3120, CachedTokens: 2048, CacheWriteTokens: 5, CompletionTokens: 96, ReasoningTokens: 40, Cost: 0.0021, CostKnown: true, FirstByte: 300 * time.Millisecond, Total: time.Second}},
		{Kind: domain.ReceiptCallStarted, At: at.Add(2 * time.Second), Call: "read-1"},
		{Kind: domain.ReceiptCallEnded, At: at.Add(3 * time.Second), Call: "read-1"},
		{Kind: domain.ReceiptRequestEnded, At: at.Add(4 * time.Second), Status: domain.ReceiptRequestCancelled, Usage: domain.UsageSample{PromptTokens: 10, Unmetered: true}},
	}
	for _, entry := range want {
		if err := store.AppendReceipt(ctx, id, entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AppendReceipt(ctx, id, domain.ReceiptEntry{Kind: "request.paused", At: at}); err == nil {
		t.Fatal("an unknown entry was journaled")
	}
	path := filepath.Join(dir, string(id)+".receipt.jsonl")
	info, err := os.Stat(path)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("journal file: %v %v", info, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString(`{"v":1,"kind":"call.sta`)
	file.Close()
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	// The next entry after the cut line is kept.
	if err := reopened.AppendReceipt(ctx, id, want[2]); err != nil {
		t.Fatal(err)
	}
	want = append(want, want[2])
	got, err := reopened.LoadReceipt(ctx, id)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v\n%v", got, want, err)
	}
	if err := os.WriteFile(path, make([]byte, maxSessionReceiptBytes-10), 0600); err != nil {
		t.Fatal(err)
	}
	if err := reopened.AppendReceipt(ctx, id, want[2]); err != errReceiptFull {
		t.Fatalf("past the bound: %v", err)
	}
}
