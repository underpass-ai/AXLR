package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// sandboxedTool records the sandbox each call ran under.
type sandboxedTool struct {
	mu   sync.Mutex
	seen []domain.SandboxProfile
}

func (t *sandboxedTool) Execute(ctx context.Context, _ domain.ToolIdentity, _ root.JSONValue) (domain.ToolOutcome, error) {
	sandbox, _ := application.ExecutionSandboxOf(ctx)
	t.mu.Lock()
	t.seen = append(t.seen, sandbox.Profile)
	t.mu.Unlock()
	return domain.ToolOutcome{Content: "done"}, nil
}

// CreateSession takes the execution's sandbox when this host can give it,
// GetSession shows it, every call of the session runs under it and the
// receipt names it; one the host cannot give, or that the contract does
// not describe, is refused with the session.
func TestCreateSessionTakesASandboxThisHostCanGive(t *testing.T) {
	model := &budgetModel{}
	tool := &sandboxedTool{}
	s, ts, client, _ := testServerWith(t, func(_ Config, deps *Dependencies) {
		deps.Start.Continue.Models = model
		deps.Start.Tools = tool
		deps.Start.Approval = approveEverything{}
		deps.Resolve.Continue.Models = model
		deps.Resolve.Tools = tool
		deps.Resolve.Approval = approveEverything{}
		deps.Tools = tool
		deps.ProbeSandbox = func(_ context.Context, sandbox domain.ExecutionSandbox) (string, error) {
			if sandbox.Profile == domain.SandboxProfileContainer {
				return "", errors.New("the profile container needs podman or docker, which is not installed")
			}
			return "sandbox process", nil
		}
	})
	for name, tc := range map[string]struct {
		body string
		code string
	}{
		"unknown field":   {`{"model":"test/model","sandbox":{"profile":"process","network":false}}`, "invalid_sandbox"},
		"empty allowlist": {`{"model":"test/model","sandbox":{"profile":"process","egress":"allowlist"}}`, "invalid_sandbox"},
		"not given here":  {`{"model":"test/model","sandbox":{"profile":"container","container":{"image":"tools:1"}}}`, "sandbox_unavailable"},
	} {
		response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", tc.body, "")
		var failure struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(response.Body).Decode(&failure)
		response.Body.Close()
		if response.StatusCode != http.StatusUnprocessableEntity || failure.Error.Code != tc.code {
			t.Fatalf("%s: status %d error %q", name, response.StatusCode, failure.Error.Code)
		}
	}
	response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"model":"test/model","sandbox":{"profile":"process","egress":"allowlist","allow":["API.github.com:443"]}}`, "")
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
	response = apiRequest(t, client, "GET", ts.URL+"/v1/sessions/"+created.ID, "", "")
	var shown struct {
		Sandbox json.RawMessage `json:"sandbox"`
	}
	if err := json.NewDecoder(response.Body).Decode(&shown); err != nil || string(shown.Sandbox) != `{"profile":"process","egress":"allowlist","allow":["api.github.com:443"]}` {
		t.Fatalf("GetSession sandbox %s: %v", shown.Sandbox, err)
	}
	response.Body.Close()
	response = apiRequest(t, client, "POST", ts.URL+"/v1/sessions/"+created.ID+"/turns", `{"prompt":"make the file","expected_revision":1}`, "turn-key-sandbox-123456")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("turn status %d", response.StatusCode)
	}
	response.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		history, _, err := s.events.Read(created.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(history); n > 0 && (history[n-1].Type == "turn.completed" || history[n-1].Type == "operation.failed" || history[n-1].Type == "turn.interrupted") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the turn did not end: %+v", history)
		}
		time.Sleep(10 * time.Millisecond)
	}
	tool.mu.Lock()
	seen := append([]domain.SandboxProfile(nil), tool.seen...)
	tool.mu.Unlock()
	if len(seen) != 1 || seen[0] != domain.SandboxProfileProcess {
		t.Fatalf("calls ran under %v", seen)
	}
	response = apiRequest(t, client, "GET", ts.URL+"/v1/sessions/"+created.ID, "", "")
	var after struct {
		Receipt *dto.ExecutionReceipt `json:"receipt"`
	}
	if err := json.NewDecoder(response.Body).Decode(&after); err != nil || after.Receipt == nil || after.Receipt.Sandbox == nil || after.Receipt.Sandbox.Profile != "process" || after.Receipt.Sandbox.Egress != "allowlist" || len(after.Receipt.Sandbox.SHA256) != 64 {
		t.Fatalf("receipt %+v: %v", after.Receipt, err)
	}
	response.Body.Close()
	// Without a probe the service confines nothing, so it takes no sandbox.
	s.deps.ProbeSandbox = nil
	response = apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"model":"test/model","sandbox":{"profile":"process"}}`, "")
	response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("a sandbox without a probe: status %d", response.StatusCode)
	}
}
