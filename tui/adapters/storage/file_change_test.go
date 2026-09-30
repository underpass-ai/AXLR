package storage

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestFileChangesSurviveSessionSaveAndStayOutsideModelHistory(t *testing.T) {
	store, _ := openStore(t)
	s := fixture(t)
	args, err := root.NewJSONObject([]byte(`{"path":"file","old_text":"old","new_text":"new"}`))
	must(t, err)
	id, err := domain.NewLocalToolIdentity("edit")
	must(t, err)
	params, err := root.NewJSONObject([]byte(`{"type":"object"}`))
	must(t, err)
	must(t, s.BeginTurn("edit the file", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "edit", Parameters: params}, Identity: id}}))
	must(t, s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "change", Name: "edit", Arguments: args}}}}))
	change := domain.FileChange{Path: "file", Before: "old\n", After: "new\n"}
	must(t, s.RecordToolOutcome("change", domain.DecisionApprove, domain.ToolOutcome{Content: "completed", Change: &change}))
	change.After = "caller mutation"
	must(t, s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "done"}}))
	must(t, store.Save(context.Background(), s))
	got, err := store.Load(context.Background(), s.Export().ID)
	must(t, err)
	preview := got.Export().Activity[0].Outcome.Change
	if preview == nil || preview.Path != "file" || preview.Before != "old\n" || preview.After != "new\n" {
		t.Fatalf("review evidence lost: %+v", preview)
	}
	preview.After = "export mutation"
	if got.Export().Activity[0].Outcome.Change.After != "new\n" {
		t.Fatal("exported preview aliases live session")
	}
	for _, m := range got.Messages() {
		if strings.Contains(string(m.Content), "old\n") || strings.Contains(string(m.Content), "new\n") {
			t.Fatal("file snapshot leaked into model messages")
		}
	}
}
