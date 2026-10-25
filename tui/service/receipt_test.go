package service

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// restartModel reads a file on its first request, hangs on its second
// until the service stops, and answers on the next, after the restart.
type restartModel struct {
	requests atomic.Int32
	hanging  chan struct{}
}

func (m *restartModel) Stream(ctx context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	cost := &root.TokenUsage{PromptTokens: 100, CompletionTokens: 5, Cost: 0.001, CostKnown: true}
	switch m.requests.Add(1) {
	case 1:
		args, _ := root.NewJSONObject([]byte(`{"path":"notes.md"}`))
		return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "read-1", Name: "local_read", Arguments: args}}}, Usage: cost, Provider: "Anthropic"}, nil
	case 2:
		close(m.hanging)
		<-ctx.Done()
		return root.CompletionResult{}, ctx.Err()
	}
	if err := emit("done"); err != nil {
		return root.CompletionResult{}, err
	}
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "done"}, Usage: cost, Provider: "Anthropic"}, nil
}

// The Phase 2 exit, in-process: a service stopped in the middle of an
// execution leaves the session interrupted with its receipt journaled; the
// restarted service resumes it with a new prompt, and the receipt lists
// every request, the one the stop cut included, and every call, with what
// each cost.
func TestReceiptSurvivesARestartOfTheService(t *testing.T) {
	model := &restartModel{hanging: make(chan struct{})}
	var cfg Config
	var deps Dependencies
	equip := func(c Config, d *Dependencies) {
		sessions, err := OpenSessions(c)
		if err != nil {
			t.Fatal(err)
		}
		d.Sessions = sessions
		for _, continuation := range []*application.ContinueTurnUseCase{&d.Start.Continue, &d.Resolve.Continue} {
			continuation.Models, continuation.Usage, continuation.Receipts = model, sessions.Usage(), sessions.Receipts()
		}
		cfg, deps = c, *d
	}
	s, ts, client, _ := testServerWith(t, equip)
	response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"model":"test/model","correlation":{"execution_id":"exec-7"}}`, "")
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("create %d: %v", response.StatusCode, err)
	}
	response.Body.Close()
	response = apiRequest(t, client, "POST", ts.URL+"/v1/sessions/"+created.ID+"/turns", `{"prompt":"read the notes","expected_revision":1}`, "turn-key-1234567890")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("turn status %d", response.StatusCode)
	}
	response.Body.Close()
	<-model.hanging
	ts.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	equip(cfg, &deps)
	restarted, err := NewServer(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	session, err := restarted.loadSession(context.Background(), created.ID)
	if err != nil || session.Status() != domain.StatusInterrupted {
		t.Fatalf("after the restart: %v %v", session.Status(), err)
	}
	before := restarted.executionReceipt(session)
	if before == nil || before.Status != "interrupted" || len(before.Requests) != 2 || before.Requests[1].Status != "cancelled" || len(before.Calls) != 1 || before.Calls[0].Status != "completed" || before.Calls[0].DurationMS == nil {
		t.Fatalf("receipt after the restart %+v", before)
	}
	// Resumed as the runbook says: an interrupted session with no pending
	// call takes a new prompt.
	restarted.runTurn(context.Background(), created.ID, "fedcba9876543210fedcba9876543210", "alice", "request-1", "continue")
	session, err = restarted.loadSession(context.Background(), created.ID)
	if err != nil || session.Status() != domain.StatusComplete {
		t.Fatalf("after the resume: %v %v", session.Status(), err)
	}
	receipt := restarted.executionReceipt(session)
	if receipt == nil || receipt.Schema != dto.ExecutionReceiptSchema || receipt.Status != "completed" || !receipt.Complete || receipt.Correlation == nil || receipt.Correlation.ExecutionID != "exec-7" {
		t.Fatalf("receipt %+v", receipt)
	}
	statuses := []string{}
	for _, request := range receipt.Requests {
		statuses = append(statuses, request.Status)
	}
	if len(receipt.Requests) != 3 || statuses[0] != "completed" || statuses[1] != "cancelled" || statuses[2] != "completed" || receipt.Requests[2].Index != 3 || receipt.Requests[0].Provider != "Anthropic" {
		t.Fatalf("requests %v %+v", statuses, receipt.Requests)
	}
	if receipt.Total.Requests != 3 || receipt.Total.CostRequests != 2 || receipt.Total.CostUSD != 0.002 || len(receipt.Calls) != 1 || receipt.Calls[0].ID != "read-1" {
		t.Fatalf("total %+v calls %+v", receipt.Total, receipt.Calls)
	}
	if result := restarted.executionResult(session, ""); result == nil || result.Usage.Requests != receipt.Total.Requests || result.Usage.CostUSD != receipt.Total.CostUSD {
		t.Fatalf("result %+v", result)
	}
}
