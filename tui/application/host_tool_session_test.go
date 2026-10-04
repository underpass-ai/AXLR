package application_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestSessionContextTitlesAfterTwoPromptsAndSurvivesResume(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "labels.json")
	labels, err := storage.NewSessionLabelStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(dir), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationSession)
	execute := func(args string) domain.ToolOutcome {
		t.Helper()
		value, err := root.NewJSONObject([]byte(args))
		if err != nil {
			t.Fatal(err)
		}
		out, err := (application.HostToolUseCase{Labels: labels}).Execute(ctx, s, identity, value)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	begin := func(prompt string) {
		t.Helper()
		if err := s.BeginTurn(root.Text(prompt), application.HostTools()); err != nil {
			t.Fatal(err)
		}
	}
	finish := func() {
		t.Helper()
		if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "done"}}); err != nil {
			t.Fatal(err)
		}
	}
	begin("Revisa AXLR")
	if out := execute(`{"title":"Auditoría AXLR"}`); !out.IsError {
		t.Fatal("premature title accepted")
	}
	if out := execute(`{"about":"project:AXLR"}`); out.IsError {
		t.Fatal(out.Content)
	}
	finish()
	begin("[AXLR] The ceremony step is still open.")
	if out := execute(`{"title":"Auditoría AXLR"}`); !out.IsError {
		t.Fatal("console reminder counted as a user exchange")
	}
	finish()
	begin("Incluye la continuidad de KMP")
	if out := execute(`{"title":"Continuidad de sesiones AXLR"}`); out.IsError {
		t.Fatal(out.Content)
	}
	finish()
	labels, err = storage.NewSessionLabelStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Title, About string
		PromptCount  int `json:"user_prompt_count"`
	}
	if err := json.Unmarshal([]byte(execute(`{}`).Content), &state); err != nil {
		t.Fatal(err)
	}
	if state.Title != "Continuidad de sesiones AXLR" || state.About != "project:AXLR" || state.PromptCount != 2 {
		t.Fatalf("resumed metadata: %+v", state)
	}
	if err := labels.Set(ctx, s.Export().ID, domain.SessionLabel{Title: "Título manual", About: "project:AXLR", Archived: true}); err != nil {
		t.Fatal(err)
	}
	if out := execute(`{"title":"Otro título","about":"project:axlr"}`); out.IsError {
		t.Fatal(out.Content)
	}
	got, err := labels.Load(ctx)
	if err != nil || got[s.Export().ID] != (domain.SessionLabel{Title: "Título manual", About: "project:AXLR", Archived: true}) {
		t.Fatalf("manual metadata overwritten: %+v %v", got, err)
	}
	for _, bad := range []string{`{"session_id":"another"}`, `{"title":"line\nbreak"}`, `{"about":" project:AXLR"}`, `{"about":null}`, `{"title":"` + strings.Repeat("á", 121) + `"}`} {
		if out := execute(bad); !out.IsError {
			t.Fatalf("accepted malformed metadata: %s", bad)
		}
	}
}
