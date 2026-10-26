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
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// gx10Routes knows a local model on this host and a remote one.
type gx10Routes struct{}

func (gx10Routes) RouteFacts(_ context.Context, model root.ModelID) domain.RouteCandidate {
	return domain.RouteCandidate{Model: model, Local: model == "qwen-local", Reachable: true}
}

func (gx10Routes) CatalogPrice(context.Context, root.ModelID) (float64, float64, bool) {
	return 1e-6, 5e-6, true
}

// downLocal fails every request to qwen-local before any sign and answers
// the others; it records the models asked.
type downLocal struct {
	mu    sync.Mutex
	asked []root.ModelID
}

func (m *downLocal) Stream(_ context.Context, req root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	m.mu.Lock()
	m.asked = append(m.asked, req.Model)
	m.mu.Unlock()
	if req.Model == "qwen-local" {
		return root.CompletionResult{}, errors.New("model endpoint 127.0.0.1:8080 transport failure")
	}
	if err := emit("done"); err != nil {
		return root.CompletionResult{}, err
	}
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "done"}}, nil
}

// CreateSession takes the execution's routing, with or without a model,
// GetSession shows it, the turn journals each model it routed to, and the
// receipt names the routing and each request's route; a routing the
// contract does not describe is refused with the session.
func TestCreateSessionTakesARouting(t *testing.T) {
	model := &downLocal{}
	s, ts, client, _ := testServerWith(t, func(_ Config, deps *Dependencies) {
		deps.Start.Continue.Models = model
		deps.Start.Continue.Routing = gx10Routes{}
		deps.Resolve.Continue.Models = model
		deps.Resolve.Continue.Routing = gx10Routes{}
	})
	for name, tc := range map[string]struct {
		body string
		code string
	}{
		"unknown field":    {`{"routing":{"candidates":["qwen-local"],"objective":"cost","routingPolicy":"cost-optimized"}}`, "invalid_routing"},
		"no objective":     {`{"routing":{"candidates":["qwen-local"]}}`, "invalid_routing"},
		"neither":          {`{}`, "invalid_model"},
		"twice the same":   {`{"routing":{"candidates":["a","a"],"objective":"cost"}}`, "invalid_routing"},
		"negative ceiling": {`{"model":"a","routing":{"candidates":["a"],"objective":"cost","max_request_usd":-1}}`, "invalid_routing"},
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
	response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"routing":{"candidates":["qwen-local","anthropic/claude-haiku-5.5"],"objective":"cost"}}`, "")
	var created struct {
		ID    string `json:"id"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil || response.StatusCode != http.StatusCreated || created.Model != "qwen-local" {
		t.Fatalf("create status %d, %+v: %v", response.StatusCode, created, err)
	}
	response.Body.Close()
	response = apiRequest(t, client, "GET", ts.URL+"/v1/sessions/"+created.ID, "", "")
	var shown struct {
		Routing json.RawMessage `json:"routing"`
	}
	if err := json.NewDecoder(response.Body).Decode(&shown); err != nil || string(shown.Routing) != `{"candidates":["qwen-local","anthropic/claude-haiku-5.5"],"objective":"cost","allow_remote_providers":true}` {
		t.Fatalf("GetSession routing %s: %v", shown.Routing, err)
	}
	response.Body.Close()
	response = apiRequest(t, client, "POST", ts.URL+"/v1/sessions/"+created.ID+"/turns", `{"prompt":"say done","expected_revision":1}`, "turn-key-routing-1234567")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("turn status %d", response.StatusCode)
	}
	response.Body.Close()
	var routed []map[string]any
	deadline := time.Now().Add(5 * time.Second)
	for {
		history, _, err := s.events.Read(created.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(history); n > 0 && (history[n-1].Type == "turn.completed" || history[n-1].Type == "operation.failed" || history[n-1].Type == "turn.interrupted") {
			for _, event := range history {
				if event.Type == "model.routed" {
					var payload map[string]any
					_ = json.Unmarshal(event.Payload, &payload)
					routed = append(routed, payload)
				}
			}
			if history[n-1].Type != "turn.completed" {
				t.Fatalf("the turn ended %s", history[n-1].Type)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the turn did not end: %+v", history)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(routed) != 2 || routed[0]["model"] != "qwen-local" || routed[0]["route"] != "ranked" || routed[1]["model"] != "anthropic/claude-haiku-5.5" || routed[1]["route"] != "fallback" || routed[1]["from"] != "qwen-local" {
		t.Fatalf("model.routed events %+v", routed)
	}
	response = apiRequest(t, client, "GET", ts.URL+"/v1/sessions/"+created.ID, "", "")
	var after struct {
		Receipt *dto.ExecutionReceipt `json:"receipt"`
	}
	if err := json.NewDecoder(response.Body).Decode(&after); err != nil || after.Receipt == nil || after.Receipt.Routing == nil || len(after.Receipt.Routing.SHA256) != 64 || after.Receipt.Routing.Objective != "cost" {
		t.Fatalf("receipt %+v: %v", after.Receipt, err)
	}
	response.Body.Close()
	// The settings' routing is the default of a session created without
	// one.
	routing, _ := domain.NewExecutionRouting([]root.ModelID{"qwen-local"}, domain.RoutingQualityFirst, false, 0, 0)
	s.deps.Routing = &routing
	response = apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{}`, "")
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil || response.StatusCode != http.StatusCreated || created.Model != "qwen-local" {
		t.Fatalf("default routing: status %d %+v", response.StatusCode, created)
	}
	response.Body.Close()
	session, err := s.loadSession(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if kept, has := session.Routing(); !has || kept.AllowsRemote() {
		t.Fatalf("the default routing was not kept: %+v %v", kept, has)
	}
}
