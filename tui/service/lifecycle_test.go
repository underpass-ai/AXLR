package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/compose"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// bigCounter counts a text with BIG in it past any cap, every other text as
// one token: the assistant's first answer pushes the session's next request
// over the cap while the successor's first request stays under it.
type bigCounter struct{}

func (bigCounter) Count(text string) int {
	if strings.Contains(text, "BIG") {
		return 1000
	}
	return 1
}

type bigModel struct{}

func (bigModel) Stream(_ context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	if err := emit("BIG answer"); err != nil {
		return root.CompletionResult{}, err
	}
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "BIG answer"}}, nil
}

func domainID(t *testing.T, id string) domain.SessionID {
	t.Helper()
	parsed, err := domain.NewSessionID(id)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func waitEvent(t *testing.T, s *Server, id, kind string) Event {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		events, _, err := s.events.Read(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Type == kind {
				return event
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %s in %+v", kind, events)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Past the context cap a service turn continues as the console's does:
// the successor belongs to the same principal and its first turn takes the
// prompt, without the client asking again (21 October 2026).
func TestCappedTurnContinuesInTheOwnersSuccessor(t *testing.T) {
	s, ts, client, _ := testServerWith(t, func(cfg Config, deps *Dependencies) {
		sessions, err := OpenSessions(cfg)
		if err != nil {
			t.Fatal(err)
		}
		deps.Sessions = sessions
		continuation := deps.Start.Continue
		continuation.Models = bigModel{}
		continuation.Tokens, continuation.Scale = bigCounter{}, &application.TokenScale{}
		continuation.ContextCap = func(root.ModelID) int { return 500 }
		continuation.Successor = compose.Successors{Create: application.CreateSessionUseCase{Store: sessions.Store()}, Store: sessions.Store()}
		deps.Start.Continue, deps.Resolve.Continue = continuation, continuation
	})
	response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"model":"test/model"}`, "")
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&created)
	response.Body.Close()
	turn := func(prompt string, revision int, key string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"prompt": prompt, "expected_revision": revision})
		response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions/"+created.ID+"/turns", string(body), key)
		if response.StatusCode != 202 {
			t.Fatalf("turn %s: %d", prompt, response.StatusCode)
		}
		response.Body.Close()
	}
	turn("hi", 1, "first-turn-key-0001")
	waitEvent(t, s, created.ID, "turn.completed")
	session, err := s.sessions.Load(context.Background(), domainID(t, created.ID))
	if err != nil {
		t.Fatal(err)
	}
	turn("again", int(session.Export().Revision), "second-turn-key-0001")
	continued := waitEvent(t, s, created.ID, "session.continued")
	var payload struct {
		Successor string `json:"successor_id"`
	}
	if err := json.Unmarshal(continued.Payload, &payload); err != nil || payload.Successor == "" {
		t.Fatalf("continued %+v %v", continued, err)
	}
	waitEvent(t, s, payload.Successor, "turn.completed")
	response = apiRequest(t, client, "GET", ts.URL+"/v1/sessions/"+payload.Successor, "", "")
	defer response.Body.Close()
	var successor struct {
		Messages []root.Message `json:"messages"`
	}
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&successor) != nil || len(successor.Messages) == 0 || !strings.Contains(string(successor.Messages[0].Content), "again") {
		t.Fatalf("successor %d %+v", response.StatusCode, successor)
	}
}

// CloseSession checks the revision, refuses a running turn and journals the
// close; without a lifecycle it records no instance.
func TestCloseSessionGuardsAndJournals(t *testing.T) {
	s, ts, client, _ := testServer(t)
	response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", `{"model":"test/model"}`, "")
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(response.Body).Decode(&created)
	response.Body.Close()
	url := ts.URL + "/v1/sessions/" + created.ID + "/close"
	if response := apiRequest(t, client, "POST", url, `{"expected_revision":9}`, ""); response.StatusCode != 409 {
		t.Fatalf("stale revision: %d", response.StatusCode)
	}
	s.registerOperation(created.ID, func() {})
	if response := apiRequest(t, client, "POST", url, `{"expected_revision":1}`, ""); response.StatusCode != 409 {
		t.Fatalf("running turn: %d", response.StatusCode)
	}
	s.finishOperation(created.ID)
	response = apiRequest(t, client, "POST", url, `{"expected_revision":1}`, "")
	if response.StatusCode != 200 {
		t.Fatalf("close: %d", response.StatusCode)
	}
	response.Body.Close()
	waitEvent(t, s, created.ID, "session.closed")
}
