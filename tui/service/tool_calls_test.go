package service

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// gatedTool blocks inside Execute until the test releases it.
type gatedTool struct {
	entered chan struct{}
	release chan struct{}
}

func (g gatedTool) Execute(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
	close(g.entered)
	<-g.release
	return domain.ToolOutcome{Content: "done"}, nil
}

func createDirectCall(t *testing.T, client *http.Client, base, arguments string) string {
	t.Helper()
	response := apiRequest(t, client, "POST", base+"/v1/tool-calls", arguments, "1234567890abcdef")
	defer response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatalf("create: %d", response.StatusCode)
	}
	var created struct {
		ID string `json:"call_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

func directCallStatus(t *testing.T, client *http.Client, base, id string) string {
	t.Helper()
	response := apiRequest(t, client, "GET", base+"/v1/tool-calls/"+id, "", "")
	defer response.Body.Close()
	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result.Status
}

func TestDirectCallResultIsAuditedBeforeCompletionIsVisible(t *testing.T) {
	s, ts, client, _ := testServer(t)
	gate := gatedTool{entered: make(chan struct{}), release: make(chan struct{})}
	s.deps.Tools = gate
	id := createDirectCall(t, client, ts.URL, `{"tool":"local_read","arguments":{"path":"sample.txt"}}`)
	response := apiRequest(t, client, "POST", ts.URL+"/v1/tool-calls/"+id+"/decisions", `{"decision":"approve","expected_revision":1}`, "abcdef1234567890")
	response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatalf("approve: %d", response.StatusCode)
	}
	<-gate.entered
	// Holding the audit lock stands in for a slow fsync of the result record.
	s.audit.mu.Lock()
	close(gate.release)
	var observed []string
	for i := 0; i < 20; i++ {
		observed = append(observed, directCallStatus(t, client, ts.URL, id))
		time.Sleep(10 * time.Millisecond)
	}
	s.audit.mu.Unlock()
	for _, status := range observed {
		if status != "running" {
			t.Fatalf("call became %s before its result was audited: %v", status, observed)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for directCallStatus(t, client, ts.URL, id) != "completed" {
		if time.Now().After(deadline) {
			t.Fatal("call did not complete after the audit resumed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	audit, err := os.ReadFile(filepath.Join(s.Config.StateDir, "audit", "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(audit), `"action":"tool_call.result"`) {
		t.Fatalf("completed call has no result record: %s", audit)
	}
}

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
