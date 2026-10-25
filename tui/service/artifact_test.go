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
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// deliveringModel delivers report.md by path on its first request and
// answers on the next.
type deliveringModel struct{ requests atomic.Int32 }

func (m *deliveringModel) Stream(_ context.Context, req root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	if m.requests.Add(1) == 1 {
		args, _ := root.NewJSONObject([]byte(`{"why":"The task asked for the report.","name":"report.md","media_type":"text/markdown","description":"The report","path":"report.md"}`))
		return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "deliver-1", Name: "axlr_deliver", Arguments: args}}}}, nil
	}
	if err := emit("delivered"); err != nil {
		return root.CompletionResult{}, err
	}
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "delivered"}}, nil
}

// waitForTurn reads the session's journal until the turn's operation ended
// and returns its events. An approval.required the loop then approved
// itself is not the end: the operation goes on.
func waitForTurn(t *testing.T, s *Server, id string) []Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		_, running := s.operations[id]
		s.mu.Unlock()
		history, _, err := s.events.Read(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(history); n > 0 && !running {
			switch history[n-1].Type {
			case "turn.completed", "turn.interrupted", "approval.required", "operation.failed":
				return history
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the turn did not finish: %+v", history)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A delivery on the service is a read the turn runs without an approver: the
// journal records an artifact event after its tool.completed, with the
// correlation, and GetSession's result lists the artifact.
func TestSessionDeliveryJournalsAnArtifact(t *testing.T) {
	model := &deliveringModel{}
	s, ts, client, _ := testServerWith(t, func(cfg Config, deps *Dependencies) {
		deps.Start.Continue.Models, deps.Start.Continue.Deliverables = model, storage.Deliverables{}
		deps.Resolve.Continue.Models, deps.Resolve.Continue.Deliverables = model, storage.Deliverables{}
		if err := os.WriteFile(filepath.Join(cfg.Workspace, "report.md"), []byte("# Report\n"), 0600); err != nil {
			t.Fatal(err)
		}
	})
	response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"model":"test/model","correlation":{"task_id":"task-1"}}`, "")
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("create %d: %v", response.StatusCode, err)
	}
	response.Body.Close()
	response = apiRequest(t, client, "POST", ts.URL+"/v1/sessions/"+created.ID+"/turns", `{"prompt":"deliver the report","expected_revision":1}`, "turn-key-1234567890")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("turn status %d", response.StatusCode)
	}
	response.Body.Close()
	history := waitForTurn(t, s, created.ID)
	var kinds []string
	for i, event := range history {
		kinds = append(kinds, event.Type)
		if event.Type != "artifact" {
			continue
		}
		var payload struct {
			Call        string                    `json:"call_id"`
			Artifact    dto.ExecutionArtifact     `json:"artifact"`
			Correlation *dto.ExecutionCorrelation `json:"correlation"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.Call != "deliver-1" || payload.Artifact.Reference != "report.md" || payload.Artifact.Size != 9 || payload.Correlation == nil || payload.Correlation.TaskID != "task-1" || history[i-1].Type != "tool.completed" {
			t.Fatalf("artifact %s: %v", event.Payload, err)
		}
	}
	if history[len(history)-1].Type != "turn.completed" || !strings.Contains(strings.Join(kinds, " "), "tool.completed artifact") {
		t.Fatalf("events %v", kinds)
	}
	response = apiRequest(t, client, "GET", ts.URL+"/v1/sessions/"+created.ID, "", "")
	var shown struct {
		Result *dto.ExecutionResult `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&shown); err != nil || shown.Result == nil || len(shown.Result.Artifacts) != 1 || shown.Result.Artifacts[0].Call != "deliver-1" || shown.Result.Artifacts[0].Source != "file" {
		t.Fatalf("GetSession %+v: %v", shown.Result, err)
	}
	response.Body.Close()
}
