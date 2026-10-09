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
	axlrruntime "github.com/underpass-ai/AXLR/runtime"
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

type approveEverything struct{}

func (approveEverything) AutoApproves(domain.ToolIdentity) bool { return true }

// reopenAudit stands in for the audit volume recovering after a failure.
func reopenAudit(t *testing.T, s *Server) {
	t.Helper()
	audit, err := newAuditStore(filepath.Join(s.Config.StateDir, "audit"))
	if err != nil {
		t.Fatal(err)
	}
	s.audit = audit
}

func TestDecisionAuditFailureLeavesCallUndecided(t *testing.T) {
	s, ts, client, _ := testServer(t)
	id := createDirectCall(t, client, ts.URL, `{"tool":"local_read","arguments":{"path":"sample.txt"}}`)
	_ = s.audit.Close() // stands in for ENOSPC or EIO on the audit volume
	url := ts.URL + "/v1/tool-calls/" + id + "/decisions"
	response := apiRequest(t, client, "POST", url, `{"decision":"approve","expected_revision":1}`, "abcdef1234567890")
	response.Body.Close()
	if response.StatusCode != 500 {
		t.Fatalf("approve without audit: %d", response.StatusCode)
	}
	call, err := s.calls.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != "pending_approval" || call.Decision != "" || call.Revision != 1 {
		t.Fatalf("unaudited decision was saved: status=%s decision=%q revision=%d", call.Status, call.Decision, call.Revision)
	}
	reopenAudit(t, s)
	response = apiRequest(t, client, "POST", url, `{"decision":"approve","expected_revision":1}`, "fresh-key-1234567890")
	response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatalf("approve after the audit recovered: %d", response.StatusCode)
	}
	deadline := time.Now().Add(2 * time.Second)
	for directCallStatus(t, client, ts.URL, id) != "completed" {
		if time.Now().After(deadline) {
			t.Fatal("approved call did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAutoApprovedCreateAuditFailureSavesNoCall(t *testing.T) {
	s, ts, client, _ := testServer(t)
	s.deps.Approval = approveEverything{}
	_ = s.audit.Close()
	response := apiRequest(t, client, "POST", ts.URL+"/v1/tool-calls", `{"tool":"local_read","arguments":{"path":"sample.txt"}}`, "1234567890abcdef")
	response.Body.Close()
	if response.StatusCode != 500 {
		t.Fatalf("create without audit: %d", response.StatusCode)
	}
	saved, err := filepath.Glob(filepath.Join(s.Config.StateDir, "tool-calls", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 0 {
		t.Fatalf("unaudited auto-approved call was saved: %v", saved)
	}
}

func TestExecuteAuditFailureNeverMarksCallRunning(t *testing.T) {
	s, ts, client, _ := testServer(t)
	gate := gatedTool{entered: make(chan struct{}), release: make(chan struct{})}
	s.deps.Tools = gate
	id := createDirectCall(t, client, ts.URL, `{"tool":"local_read","arguments":{"path":"sample.txt"}}`)
	// Occupy every direct slot so the approved call cannot start yet.
	for i := 0; i < cap(s.directSlots); i++ {
		s.directSlots <- struct{}{}
	}
	response := apiRequest(t, client, "POST", ts.URL+"/v1/tool-calls/"+id+"/decisions", `{"decision":"approve","expected_revision":1}`, "abcdef1234567890")
	response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatalf("approve: %d", response.StatusCode)
	}
	lock := s.callLock(id)
	lock.Lock()
	_ = s.audit.Close()
	for i := 0; i < cap(s.directSlots); i++ {
		<-s.directSlots
	}
	// The worker holds a slot while it waits for the call lock.
	for len(s.directSlots) != 1 {
		time.Sleep(time.Millisecond)
	}
	lock.Unlock()
	// It returns the slot only after it has finished.
	for len(s.directSlots) != 0 {
		time.Sleep(time.Millisecond)
	}
	select {
	case <-gate.entered:
		t.Fatal("tool ran without an execute record")
	default:
	}
	call, err := s.calls.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != "pending_approval" || call.Decision != "approve" || call.Revision != 2 {
		t.Fatalf("call that never ran is reported as status=%s revision=%d", call.Status, call.Revision)
	}
}

// deadlineTool reports how long its context leaves the call to run.
type deadlineTool struct{ remaining chan time.Duration }

func (d deadlineTool) Execute(ctx context.Context, _ domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		d.remaining <- -1
	} else {
		d.remaining <- time.Until(deadline)
	}
	return domain.ToolOutcome{Content: "done"}, nil
}

func TestApprovedExecRunsUnderItsOwnTimeout(t *testing.T) {
	s, ts, client, _ := testServer(t)
	tool := deadlineTool{remaining: make(chan time.Duration, 1)}
	s.deps.Tools = tool
	id := createDirectCall(t, client, ts.URL, `{"tool":"local_exec","arguments":{"program":"/bin/sleep","args":["120"],"timeout_ms":180000}}`)
	response := apiRequest(t, client, "POST", ts.URL+"/v1/tool-calls/"+id+"/decisions", `{"decision":"approve","expected_revision":1}`, "abcdef1234567890")
	response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatalf("approve: %d", response.StatusCode)
	}
	select {
	case remaining := <-tool.remaining:
		if remaining <= 180*time.Second || remaining > axlrruntime.HardTimeout+time.Minute {
			t.Fatalf("exec with timeout_ms=180000 runs under a %v deadline", remaining.Round(time.Second))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tool did not run")
	}
}
