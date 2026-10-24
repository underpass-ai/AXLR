package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// A session's budget account and correlation are saved beside its snapshot
// and come back with it; a file the store cannot read refuses the session.
func TestSessionStoreKeepsTheBudgetAndCorrelation(t *testing.T) {
	store, dir := openStore(t)
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	session, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	budget, err := domain.NewExecutionBudget(900, 20, 60, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
	if err := session.SetBudget(budget, started, domain.UsageLedger{Requests: 2, Cost: 0.1}); err != nil {
		t.Fatal(err)
	}
	if err := session.StopAtBudget(domain.BudgetRequests); err != nil {
		t.Fatal(err)
	}
	correlation := domain.ExecutionCorrelation{BotID: "server-incident-investigator", TaskID: "task-789", ExecutionID: "exec-1", TraceID: "trace-abc"}
	if err := session.SetCorrelation(correlation); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, string(id)+".execution")); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	account, has := loaded.Budget()
	if !has || account.Budget != budget || !account.StartedAt.Equal(started) || account.Base != (domain.ExecutionSpend{Requests: 2, USD: 0.1}) || account.Stopped != domain.BudgetRequests {
		t.Fatalf("%+v %v", account, has)
	}
	if kept, has := loaded.Correlation(); !has || kept != correlation {
		t.Fatalf("%+v %v", kept, has)
	}
	loaded.ClearBudget()
	loaded.ClearCorrelation()
	if err := store.Save(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, string(id)+".execution")); !os.IsNotExist(err) {
		t.Fatalf("the execution file stays after clearing: %v", err)
	}
	again, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := again.Budget(); has {
		t.Fatal("a cleared budget came back")
	}
	if _, has := again.Correlation(); has {
		t.Fatal("a cleared correlation came back")
	}
	for name, raw := range map[string]string{
		"unknown version": `{"version":2}`,
		"unknown field":   `{"version":1,"budget":{"limits":{"minutes":1},"started_at":"2026-10-24T10:00:00Z","carried":{"requests":0,"tool_calls":0,"usd":0},"base":{"requests":0,"tool_calls":0,"usd":0}}}`,
		"unknown stop":    `{"version":1,"budget":{"limits":{"requests":1},"started_at":"2026-10-24T10:00:00Z","carried":{"requests":0,"tool_calls":0,"usd":0},"base":{"requests":0,"tool_calls":0,"usd":0},"stopped":"minutes"}}`,
		"unsafe id":       `{"version":1,"correlation":{"bot_id":"bot one"}}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, string(id)+".execution"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(ctx, id); err == nil {
			t.Fatalf("%s: a session with an unreadable execution file loaded", name)
		}
	}
	if err := os.Remove(filepath.Join(dir, string(id)+".execution")); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
}
