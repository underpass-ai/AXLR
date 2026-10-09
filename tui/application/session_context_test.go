package application

import (
	"context"
	"encoding/json"
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
		if strings.Contains(string(req.Messages[0].Content), "user_prompt_count") {
			t.Fatal("the system prompt carries a value that changes every turn")
		}
		if calls == 1 {
			return assistant("", root.ToolCall{ID: "set-title", Name: HostSessionName, Arguments: hostJSON(t, `{"title":"Corregir sesiones AXLR","about":"project:AXLR"}`)}), nil
		}
		// The title stays out of the system prompt, whose prefix the cache
		// reuses; axlr_session reports it.
		if strings.Contains(string(req.Messages[0].Content), "Corregir sesiones AXLR") {
			t.Fatal("the title changed the system prompt")
		}
		if labels.labels[s.Export().ID].Title != "Corregir sesiones AXLR" {
			t.Fatal("new title not stored")
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

// A prefix cache reuses nothing after the first changed token, so the system
// prompt must not change from one user turn to the next while the session's
// labels stay the same.
func TestSystemPromptStaysIdenticalAcrossUserTurns(t *testing.T) {
	s := turnSession(t)
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}
	var prompts []root.Text
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, SessionLabels: labels, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		prompts = append(prompts, req.Messages[0].Content)
		return assistant("Hecho"), nil
	})}}
	for _, prompt := range []string{"Revisa AXLR", "Ahora el skill", "Y los tests"} {
		if err := s.BeginTurn(root.Text(prompt), HostTools()); err != nil {
			t.Fatal(err)
		}
		if err := u.Execute(context.Background(), &s, nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(prompts) != 3 || prompts[0] != prompts[1] || prompts[1] != prompts[2] {
		t.Fatalf("system prompt changed between user turns:\n%s\n---\n%s", prompts[0], prompts[len(prompts)-1])
	}
}

type failedRecall struct{ *fakeMemory }

func (failedRecall) Wake(context.Context, string) (string, error) {
	return "", errors.New("guide not installed")
}

func (failedRecall) WakeFocused(context.Context, string, string) (string, []string, error) {
	return "", nil, errors.New("guide not installed")
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

// The system prompt asks to establish the memory scope early: a first-prompt
// call that also proposes a title keeps the about and defers only the title.
func TestAnEarlyTitleIsDeferredWithoutLosingTheAbout(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("revisa", HostTools()); err != nil {
		t.Fatal(err)
	}
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationSession)
	out, err := (HostToolUseCase{Labels: labels}).Execute(context.Background(), s, identity, hostJSON(t, `{"title":"Revisar AXLR","about":"project:AXLR"}`))
	if err != nil || out.IsError {
		t.Fatalf("refused whole: %v %s", err, out.Content)
	}
	if stored := labels.labels[s.Export().ID]; stored.About != "project:AXLR" || stored.Title != "" {
		t.Fatalf("stored label: %+v", stored)
	}
	var result struct {
		Title    string `json:"title"`
		About    string `json:"about"`
		Deferred string `json:"title_deferred"`
	}
	if err := json.Unmarshal([]byte(out.Content), &result); err != nil {
		t.Fatal(err)
	}
	if result.About != "project:AXLR" || result.Title != "" || !strings.Contains(result.Deferred, "second user prompt") {
		t.Fatalf("result: %s", out.Content)
	}
}

// Console messages all start with "[AXLR": the memory reminder and Jev's
// "[AXLR · …]" ones are not the person's prompts either.
func TestConsoleMessagesDoNotCountAsUserPrompts(t *testing.T) {
	for _, console := range []root.Text{memoryReminder, jevFinalPrefix + " ...", "[AXLR] The build step of ceremony axlr_delivery is still open."} {
		s := turnSession(t)
		if err := s.BeginTurn("arregla el test", HostTools()); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAssistant(assistant("hecho")); err != nil {
			t.Fatal(err)
		}
		if err := s.BeginTurn(console, HostTools()); err != nil {
			t.Fatal(err)
		}
		if count := userPromptCount(s); count != 1 {
			t.Fatalf("%.20q counted: %d prompts", console, count)
		}
	}
}
