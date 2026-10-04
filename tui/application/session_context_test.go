package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type contextLabels struct {
	labels map[domain.SessionID]domain.SessionLabel
}

func (l *contextLabels) Load(context.Context) (map[domain.SessionID]domain.SessionLabel, error) {
	return l.labels, nil
}
func (l *contextLabels) Set(_ context.Context, id domain.SessionID, label domain.SessionLabel) error {
	l.labels[id] = label
	return nil
}
func (l *contextLabels) Initialize(_ context.Context, id domain.SessionID, label domain.SessionLabel) (domain.SessionLabel, error) {
	old := l.labels[id]
	if old.Title == "" {
		old.Title = label.Title
	}
	if old.About == "" {
		old.About = label.About
	}
	l.labels[id] = old
	return old, nil
}

func TestSecondExchangeAutomaticallyPersistsTitleInAgentLoop(t *testing.T) {
	s := turnSession(t)
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}
	store := &memoryStore{}
	if err := s.BeginTurn("Revisa AXLR", HostTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("Encontré el problema")); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("Corrige el skill de sesiones", HostTools()); err != nil {
		t.Fatal(err)
	}
	calls := 0
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: store, SessionLabels: labels, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		calls++
		if !strings.Contains(string(req.Messages[0].Content), `"user_prompt_count":2`) {
			t.Fatal("lost original prompt count")
		}
		if calls == 1 {
			return assistant("", root.ToolCall{ID: "set-title", Name: HostSessionName, Arguments: hostJSON(t, `{"title":"Corregir sesiones AXLR","about":"project:AXLR"}`)}), nil
		}
		if !strings.Contains(string(req.Messages[0].Content), `"title":"Corregir sesiones AXLR"`) {
			t.Fatal("new title not reflected in model context")
		}
		return assistant("Corregido"), nil
	})}}
	if err := u.Execute(context.Background(), &s, nil); err != nil {
		t.Fatal(err)
	}
	activity := s.Export().Activity
	if calls != 2 || s.Status() != domain.StatusComplete || len(activity) != 1 || activity[0].Decision != domain.DecisionAutoApprove || activity[0].Outcome.IsError {
		t.Fatalf("bookkeeping interrupted normal work: %+v", activity)
	}
}

type failedRecall struct{ *fakeMemory }

func (failedRecall) Wake(context.Context, string) (string, error) {
	return "", errors.New("guide not installed")
}

func TestCeremonyUsesSavedProjectAndScopeChosenAfterBegin(t *testing.T) {
	for _, early := range []bool{true, false} {
		t.Run(map[bool]string{true: "before-begin", false: "after-begin"}[early], func(t *testing.T) {
			s := debugSession(t)
			labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}
			if early {
				labels.labels[s.Export().ID] = domain.SessionLabel{About: "project:AXLR"}
			}
			memory := &fakeMemory{}
			d := CeremonyDriver{Engine: &fakeEngine{}, Checks: &fakeChecks{}, Memory: failedRecall{memory}, Labels: labels}
			if err := d.Begin(context.Background(), &s, "repair"); err != nil {
				t.Fatal(err)
			}
			run, _ := s.Ceremony()
			want := "ws:" + string(s.Export().ID)
			if early {
				want = "project:AXLR"
			}
			if run.About != want || !strings.Contains(run.Memory, "guide not installed") {
				t.Fatalf("lost recall provenance: %+v", run)
			}
			labels.labels[s.Export().ID] = domain.SessionLabel{About: "project:AXLR"}
			if got := d.record(context.Background(), s, run, "BLOCKED", nil); got != "recorded in project:AXLR" || memory.about != "project:AXLR" {
				t.Fatalf("terminal outcome scope: %s %+v", got, memory)
			}
		})
	}
}
