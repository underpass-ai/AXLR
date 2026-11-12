package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// strictRemember is the axlr_remember call a strict-schema model sent on
// 12 November 2026: every field filled, an invented about meaning "none".
func strictRemember(n int) root.ToolCall {
	about := "project:dev.void"
	if n > 1 {
		about += fmt.Sprint(n)
	}
	return argued(root.ToolCallID(fmt.Sprintf("call_%d", n)), HostRememberName, `{"kind":"observation","text":"ready","summary_en":"ready","evidence":"e","links":[],"about":"`+about+`","continuation":" "}`)
}

// repeatingRemember is a model that answers every request with the next
// strictRemember, keeping each request's axlr_remember schema.
func repeatingRemember(requests *int, schemas *[]string) streamFunc {
	return func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		*requests++
		for _, tool := range req.Tools {
			if tool.Name == HostRememberName {
				*schemas = append(*schemas, string(tool.Parameters.Bytes()))
			}
		}
		if *requests > 6 {
			return assistant("done"), nil
		}
		return assistant("", strictRemember(*requests)), nil
	}
}

// A model that sends axlr_remember again and again with another invented
// about, each refused alike, is stopped at the third refusal: the turn is
// interrupted and kept, the execution fails with the tool loop named, and
// no fourth request is made. On 12 November 2026 a dev bot sent it ten
// times until someone cancelled the run by hand.
func TestARunStopsAModelThatRepeatsTheSameFailure(t *testing.T) {
	s := boundSession(t, true)
	recorder := &rememberRecorder{answer: map[string]any{"accepted": true}}
	requests, schemas := 0, []string{}
	store := &memoryStore{}
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store, Remember: recorder, SessionLabels: &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}, Models: repeatingRemember(&requests, &schemas)},
		Approval: approvalFunc(func(domain.ToolIdentity) bool { return true })}
	u.Continue.ToolFailureStreak = 3
	err := u.Execute(context.Background(), &s, ignoreEvent)
	var stop *domain.ToolFailureStreakError
	if !errors.As(err, &stop) || stop.Tool != HostRememberName || stop.Count != 3 {
		t.Fatalf("err %v", err)
	}
	if requests != 3 || len(recorder.requests) != 0 {
		t.Fatalf("%d requests, %d writes", requests, len(recorder.requests))
	}
	if s.Status() != domain.StatusInterrupted || store.states[len(store.states)-1].Status != domain.StatusInterrupted {
		t.Fatalf("status %s, saved %s", s.Status(), store.states[len(store.states)-1].Status)
	}
	if !strings.Contains(stop.Failure, "project:dev.void3") || !strings.HasPrefix(stop.Failure, "leave about out to write under ") {
		t.Fatalf("failure %q", stop.Failure)
	}
	result := domain.NewExecutionResult(s, err)
	if result.Status != domain.ExecutionFailed || !strings.HasPrefix(result.Error, "tool loop: axlr_remember failed 3 times in a row with the same error") {
		t.Fatalf("result %+v", result)
	}
	// Every request offered axlr_remember with the abouts the policy
	// lets the bot write, its own first.
	state := s.Export()
	project, own := "project:"+filepath.Base(string(state.Workspace)), "ws:"+string(state.ID)
	for _, schema := range schemas {
		var parsed struct {
			Properties map[string]struct {
				Enum        []string `json:"enum"`
				Description string   `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal([]byte(schema), &parsed); err != nil {
			t.Fatal(err)
		}
		about := parsed.Properties["about"]
		if strings.Join(about.Enum, ",") != project+","+own || !strings.HasPrefix(about.Description, "Leave it out: the memory goes under "+project+".") {
			t.Fatalf("about %+v", about)
		}
		if !strings.Contains(parsed.Properties["continuation"].Description, "leave it out otherwise") {
			t.Fatalf("continuation %+v", parsed.Properties["continuation"])
		}
	}
	if len(schemas) != 3 {
		t.Fatalf("%d schemas", len(schemas))
	}
}

// The console leaves the streak at zero: a person reads each failure, and
// the turn goes on until the model answers.
func TestTheConsoleLetsTheModelGoOnAfterRepeatedFailures(t *testing.T) {
	s := boundSession(t, true)
	requests, schemas := 0, []string{}
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, Remember: &rememberRecorder{}, SessionLabels: &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}, Models: repeatingRemember(&requests, &schemas)},
		Approval: approvalFunc(func(domain.ToolIdentity) bool { return true })}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil || s.Status() != domain.StatusComplete || requests != 7 {
		t.Fatalf("%v %s %d", err, s.Status(), requests)
	}
}

// A session whose memory no policy bounds keeps axlr_remember's schema as
// it is: any about is its to name.
func TestAnUnboundSessionKeepsRemembersOpenAbout(t *testing.T) {
	s := rememberSession(t, domain.ModeNormal)
	tools := []root.ToolDefinition{RememberTool().Definition}
	if got := withRememberAbouts(tools, s); string(got[0].Parameters.Bytes()) != rememberSchema {
		t.Fatalf("schema %s", got[0].Parameters.Bytes())
	}
	bound := boundSession(t, true)
	got := withRememberAbouts(tools, bound)
	if string(tools[0].Parameters.Bytes()) != rememberSchema || !strings.Contains(string(got[0].Parameters.Bytes()), `"enum":[`) {
		t.Fatalf("the snapshot's schema changed or the request's did not: %s", got[0].Parameters.Bytes())
	}
	if _, err := rememberParameters(nil); err == nil {
		t.Fatal("no abouts gave a schema")
	}
}
