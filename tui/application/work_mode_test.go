package application

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type allowEverything struct{}

func localSnapshot(t *testing.T) []domain.AvailableTool {
	t.Helper()
	snapshot := HostTools()
	for _, op := range []string{"read", "write", "edit", "exec"} {
		id, err := domain.NewLocalToolIdentity(op)
		if err != nil {
			t.Fatal(err)
		}
		snapshot = append(snapshot, domain.AvailableTool{Definition: root.ToolDefinition{Name: root.ToolName("local_" + op), Description: "x", Parameters: mustObject(t, `{"type":"object"}`)}, Identity: id})
	}
	return snapshot
}

// pendingCall returns a session in the given mode awaiting approval of one
// local call, and the store the rejection path saves into.
func pendingCall(t *testing.T, mode domain.WorkMode, name, arguments string) (domain.Session, *memoryStore) {
	t.Helper()
	s := turnSession(t)
	if err := s.SetMode(mode); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("haz el cambio", localSnapshot(t)); err != nil {
		t.Fatal(err)
	}
	pending := root.ToolCall{ID: "call-1", Name: root.ToolName(name), Arguments: mustObject(t, arguments)}
	if err := s.CompleteAssistant(assistant("", pending)); err != nil {
		t.Fatal(err)
	}
	if s.Status() != domain.StatusApproval || len(s.Pending()) != 1 {
		t.Fatalf("fixture is not awaiting approval: %s", s.Status())
	}
	return s, &memoryStore{}
}

func (allowEverything) AutoApproves(domain.ToolIdentity) bool { return true }

func TestReviewModeRejectsWritesWithAModelVisibleReason(t *testing.T) {
	s, store := pendingCall(t, domain.ModeReview, "local_write", `{"path":"wc.py","content":"x"}`)
	if err := rejectUnknown(context.Background(), &s, store, ignoreEvent, nil); err != nil {
		t.Fatal(err)
	}
	activity := s.Export().Activity
	outcome := activity[len(activity)-1].Outcome
	if outcome == nil || !outcome.IsError || !strings.HasPrefix(string(outcome.Content), "denied by review mode: ") {
		t.Fatalf("write was not refused by mode: %+v", outcome)
	}
}

func TestWriterModeLetsDocumentsThrough(t *testing.T) {
	s, store := pendingCall(t, domain.ModeWriter, "local_write", `{"path":"docs/usage.md","content":"x"}`)
	if err := rejectUnknown(context.Background(), &s, store, ignoreEvent, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.Pending()) != 1 {
		t.Fatal("a document write was refused")
	}
}

func TestWriterModeRejectsNonDocumentWritesWithAModelVisibleReason(t *testing.T) {
	s, store := pendingCall(t, domain.ModeWriter, "local_write", `{"path":"wc.py","content":"x"}`)
	if err := rejectUnknown(context.Background(), &s, store, ignoreEvent, nil); err != nil {
		t.Fatal(err)
	}
	activity := s.Export().Activity
	outcome := activity[len(activity)-1].Outcome
	if outcome == nil || !outcome.IsError || !strings.HasPrefix(string(outcome.Content), "denied by writer mode: ") {
		t.Fatalf("non-document write was not refused by mode: %+v", outcome)
	}
}

func TestWriterModeRejectsPathTraversalOutOfDocs(t *testing.T) {
	s, store := pendingCall(t, domain.ModeWriter, "local_write", `{"path":"docs/../wc.py","content":"x"}`)
	if err := rejectUnknown(context.Background(), &s, store, ignoreEvent, nil); err != nil {
		t.Fatal(err)
	}
	activity := s.Export().Activity
	outcome := activity[len(activity)-1].Outcome
	if outcome == nil || !outcome.IsError || !strings.HasPrefix(string(outcome.Content), "denied by writer mode: ") {
		t.Fatalf("path traversal out of docs/ was not refused by mode: %+v", outcome)
	}
}

func TestHumanApprovalOfAModeDeniedCallIsStillRejected(t *testing.T) {
	s, store := pendingCall(t, domain.ModeReview, "local_write", `{"path":"wc.py","content":"x"}`)
	u := ResolveToolUseCase{
		Store: store,
		Tools: executionFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
			t.Fatal("mode-denied call executed despite human approval")
			return domain.ToolOutcome{}, nil
		}),
		Continue: ContinueTurnUseCase{Store: store, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
			return assistant("done"), nil
		})},
	}
	if err := u.Execute(context.Background(), &s, "call-1", domain.DecisionApprove, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	outcome := s.Export().Activity[0].Outcome
	if outcome == nil || !outcome.IsError || !strings.HasPrefix(string(outcome.Content), "denied by review mode: ") {
		t.Fatalf("human approval bypassed mode denial: %+v", outcome)
	}
}

func TestModesKeepExecUnderHumanApprovalEvenWithAutonomy(t *testing.T) {
	exec, _ := domain.NewLocalToolIdentity("exec")
	args, _ := root.NewJSONObject([]byte(`{"program":"ls"}`))
	if !approvesInMode(allowEverything{}, domain.ModeNormal, exec, args) {
		t.Fatal("normal mode lost autonomy")
	}
	for _, mode := range []domain.WorkMode{domain.ModeReview, domain.ModeWriter, domain.ModeResearch} {
		if approvesInMode(allowEverything{}, mode, exec, args) {
			t.Fatalf("%s auto-approved exec", mode)
		}
	}
}

func TestReviewModeHidesWriteToolsFromTheModel(t *testing.T) {
	snapshot := localSnapshot(t)
	names := func(mode domain.WorkMode) string {
		var out []string
		for _, tool := range ModeTools(mode, snapshot) {
			out = append(out, string(tool.Name))
		}
		return strings.Join(out, ",")
	}
	if strings.Contains(names(domain.ModeReview), "local_write") || strings.Contains(names(domain.ModeReview), "local_edit") {
		t.Fatal("review mode exposes write tools")
	}
	if !strings.Contains(names(domain.ModeWriter), "local_write") || !strings.Contains(names(domain.ModeReview), "local_read") {
		t.Fatal("mode removed the wrong tools")
	}
}

func TestGuidanceDescribesTheActiveModeAndNoLongerRoutesToTheSkill(t *testing.T) {
	for _, mode := range []domain.WorkMode{domain.ModeNormal, domain.ModeReview, domain.ModeWriter, domain.ModeResearch} {
		s := turnSession(t)
		if err := s.SetMode(mode); err != nil {
			t.Fatal(err)
		}
		text := string(modelHostGuidance(&s).Content)
		if strings.Contains(text, "axlr-ceremonies") {
			t.Fatalf("%s guidance still routes work to the 1.0 skill", mode)
		}
		if mode != domain.ModeNormal && !strings.Contains(text, "Mode: "+string(mode)+".") {
			t.Fatalf("%s guidance missing", mode)
		}
	}
}

func mustObject(t *testing.T, raw string) root.JSONValue {
	t.Helper()
	value, err := root.NewJSONObject([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return value
}
