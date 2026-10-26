package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
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

// A session's sandbox is saved beside its snapshot and comes back with it;
// one the store cannot read refuses the session, so a lost sandbox never
// lets the next command run unconfined.
func TestSessionStoreKeepsTheSandbox(t *testing.T) {
	store, dir := openStore(t)
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	session, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	sandbox := domain.ExecutionSandbox{Profile: domain.SandboxProfileContainer, Egress: domain.SandboxEgressAllowlist, Allow: []string{"api.github.com:443"}, Writable: []string{"/cache"}, Container: &domain.SandboxContainer{Image: "tools:1", MemoryMB: 512, CPUs: 1.5, PIDs: 64}}
	if err := session.SetSandbox(sandbox); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	kept, has := loaded.Sandbox()
	if !has || kept.Profile != sandbox.Profile || kept.Egress != sandbox.Egress || len(kept.Allow) != 1 || kept.Allow[0] != "api.github.com:443" || len(kept.Writable) != 1 || kept.Container == nil || *kept.Container != *sandbox.Container {
		t.Fatalf("%+v %v", kept, has)
	}
	loaded.ClearSandbox()
	if err := store.Save(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, string(id)+".execution")); !os.IsNotExist(err) {
		t.Fatalf("the execution file stays after clearing: %v", err)
	}
	for name, raw := range map[string]string{
		"unknown profile":  `{"version":1,"sandbox":{"profile":"vm","egress":"none"}}`,
		"empty allowlist":  `{"version":1,"sandbox":{"profile":"process","egress":"allowlist"}}`,
		"unknown field":    `{"version":1,"sandbox":{"profile":"process","egress":"none","network":true}}`,
		"container absent": `{"version":1,"sandbox":{"profile":"container","egress":"none"}}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, string(id)+".execution"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(ctx, id); err == nil {
			t.Fatalf("%s: a session with an unreadable sandbox loaded", name)
		}
	}
}

func TestSessionStoreKeepsTheRouting(t *testing.T) {
	store, dir := openStore(t)
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	session, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "qwen-local")
	if err != nil {
		t.Fatal(err)
	}
	routing, err := domain.NewExecutionRouting([]root.ModelID{"qwen-local", "anthropic/claude-haiku-5.5"}, domain.RoutingCost, false, 32000, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.SetRouting(routing); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	kept, has := loaded.Routing()
	if !has || kept.Objective() != domain.RoutingCost || kept.AllowsRemote() || kept.MaxContextTokens() != 32000 || kept.MaxRequestUSD() != 0.05 || len(kept.Candidates()) != 2 {
		t.Fatalf("%+v %v", kept, has)
	}
	for name, raw := range map[string]string{
		"no candidate":  `{"version":1,"routing":{"candidates":[],"objective":"cost"}}`,
		"objective":     `{"version":1,"routing":{"candidates":["a"],"objective":"cheap"}}`,
		"unknown field": `{"version":1,"routing":{"candidates":["a"],"objective":"cost","fallback":true}}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, string(id)+".execution"), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(ctx, id); err == nil {
			t.Fatalf("%s: a session with an unreadable routing loaded", name)
		}
	}
}
