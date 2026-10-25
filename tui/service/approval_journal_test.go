package service

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// callingModel makes its one call, call-1, on its first request and
// answers on the next.
type callingModel struct {
	tool      root.ToolName
	arguments string
	requests  atomic.Int32
}

// readingModel reads a file: a read the console approves by itself.
func readingModel() *callingModel {
	return &callingModel{tool: "local_read", arguments: `{"path":"sample.txt"}`}
}

func (m *callingModel) Stream(_ context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	if m.requests.Add(1) == 1 {
		args, _ := root.NewJSONObject([]byte(m.arguments))
		return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call-1", Name: m.tool, Arguments: args}}}}, nil
	}
	if err := emit("done it"); err != nil {
		return root.CompletionResult{}, err
	}
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "done it"}}, nil
}

// startCallingTurn creates a session and starts the turn whose model makes
// one call; it returns the session's id.
func startCallingTurn(t *testing.T, client *http.Client, base string) string {
	t.Helper()
	response := apiRequest(t, client, "POST", base+"/v1/sessions", `{"model":"test/model"}`, "")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", response.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	response = apiRequest(t, client, "POST", base+"/v1/sessions/"+created.ID+"/turns", `{"prompt":"do it","expected_revision":1}`, "turn-key-1234567890")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("turn status %d", response.StatusCode)
	}
	response.Body.Close()
	return created.ID
}

// journalUntil reads the session's journal until its last event is one of
// last, and returns it.
func journalUntil(t *testing.T, s *Server, id string, last ...string) []Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		history, _, err := s.events.Read(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) > 0 {
			for _, kind := range last {
				if history[len(history)-1].Type == kind {
					return history
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the journal did not reach %v: %v", last, eventKinds(history))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func eventKinds(history []Event) []string {
	kinds := make([]string, 0, len(history))
	for _, event := range history {
		kinds = append(kinds, event.Type)
	}
	return kinds
}

// A call the console approves by itself, a read, runs without a decision: the
// journal says tool.requested then tool.completed, and no approval.required
// claims a decision is due.
func TestAutoApprovedCallJournalsNoDecision(t *testing.T) {
	model := readingModel()
	s, ts, client, _ := testServerWith(t, func(_ Config, deps *Dependencies) {
		deps.Start.Continue.Models = model
		deps.Resolve.Continue.Models = model
	})
	id := startCallingTurn(t, client, ts.URL)
	history := journalUntil(t, s, id, "turn.completed", "operation.failed", "approval.required")
	kinds := eventKinds(history)
	if history[len(history)-1].Type != "turn.completed" || strings.Contains(strings.Join(kinds, " "), "approval.required") {
		t.Fatalf("journal %v", kinds)
	}
	if !strings.Contains(strings.Join(kinds, " "), "tool.requested tool.completed") {
		t.Fatalf("the call is not journaled requested then completed: %v", kinds)
	}
}

// A call that waits for a decision ends its operation at one
// approval.required, the last event, whose result names the call; the
// decision's operation runs it and journals no other.
func TestCallThatWaitsJournalsOneDecision(t *testing.T) {
	// A command waits under no approval policy; the tool only answers.
	model := &callingModel{tool: "local_exec", arguments: `{"program":"touch","args":["made-by-service"]}`}
	s, ts, client, _ := testServerWith(t, func(_ Config, deps *Dependencies) {
		deps.Start.Continue.Models = model
		deps.Resolve.Continue.Models = model
	})
	id := startCallingTurn(t, client, ts.URL)
	history := journalUntil(t, s, id, "approval.required", "turn.completed", "operation.failed")
	terminal := history[len(history)-1]
	var payload struct {
		Status string               `json:"status"`
		Result *dto.ExecutionResult `json:"result"`
	}
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil || terminal.Type != "approval.required" || payload.Status != "approval" || payload.Result == nil || payload.Result.Status != "needs_decision" || payload.Result.Pending == nil || payload.Result.Pending.ID != "call-1" {
		t.Fatalf("%s %s: %v (journal %v)", terminal.Type, terminal.Payload, err, eventKinds(history))
	}
	stored, err := s.sessions.Load(context.Background(), domain.SessionID(id))
	if err != nil {
		t.Fatal(err)
	}
	response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions/"+id+"/approvals/call-1", `{"decision":"approve","expected_revision":`+jsonNumber(stored.Export().Revision)+`}`, "approval-key-123456")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("approval status %d", response.StatusCode)
	}
	response.Body.Close()
	history = journalUntil(t, s, id, "turn.completed", "operation.failed", "approval.required")
	if kinds := eventKinds(history); history[len(history)-1].Type != "turn.completed" || strings.Count(strings.Join(kinds, " "), "approval.required") != 1 {
		t.Fatalf("journal %v", kinds)
	}
}

func jsonNumber(n uint64) string {
	data, _ := json.Marshal(n)
	return string(data)
}

// A crash while an approved call runs leaves the journal at the call, not
// at a decision: the next start journals turn.interrupted for the turn.
// The crash is the state directory as it stands while the tool runs,
// opened by a new server.
func TestCrashDuringAToolIsInterruptedAtRestart(t *testing.T) {
	model := readingModel()
	gate := gatedTool{entered: make(chan struct{}), release: make(chan struct{})}
	s, ts, client, _ := testServerWith(t, func(_ Config, deps *Dependencies) {
		deps.Start.Continue.Models = model
		deps.Start.Tools = gate
		deps.Resolve.Continue.Models = model
		deps.Resolve.Tools = gate
	})
	defer close(gate.release)
	id := startCallingTurn(t, client, ts.URL)
	select {
	case <-gate.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the tool did not run")
	}
	crashed := s.Config
	crashed.StateDir = filepath.Join(t.TempDir(), "state")
	if err := os.CopyFS(crashed.StateDir, os.DirFS(s.Config.StateDir)); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewServer(crashed, Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	history, _, err := restarted.events.Read(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	kinds := eventKinds(history)
	if len(history) < 2 || history[len(history)-2].Type != "tool.requested" || history[len(history)-1].Type != "turn.interrupted" || history[len(history)-1].OperationID != history[0].OperationID {
		t.Fatalf("journal after restart %v", kinds)
	}
	recovered, err := restarted.sessions.Load(context.Background(), domain.SessionID(id))
	if err != nil || recovered.Status() != domain.StatusInterrupted {
		t.Fatalf("recovered status %s: %v", recovered.Status(), err)
	}
}
