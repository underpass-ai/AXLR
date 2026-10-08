package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A remote model's request stays within the prompt budget: a session of
// closed turns that the 1 MiB ceiling kept whole is cut, with a checkpoint,
// to the prompt budget's ceiling.
func TestContinueTurnBoundsAnUnknownWindowToThePromptBudget(t *testing.T) {
	s := turnSession(t)
	for i := 0; i < 12; i++ {
		if err := s.BeginTurn(root.Text(strings.Repeat("palabra ", 2000)), turnTools()); err != nil { // 16 KB
			t.Fatal(err)
		}
		if err := s.CompleteAssistant(assistant("listo")); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.BeginTurn("sigue", turnTools()); err != nil {
		t.Fatal(err)
	}
	var sent root.CompletionRequest
	u := ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		sent = req
		return assistant("ok"), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	budget := domain.ContextBudgetForPrompt(domain.DefaultPromptTokens)
	if bytes := ModelMessagesBytes(sent.Messages[1:]); bytes > budget.MaximumBytes() || bytes <= budget.LowWaterBytes()/2 {
		t.Fatalf("projected %d bytes against a budget of %d/%d", bytes, budget.MaximumBytes(), budget.LowWaterBytes())
	}
	if !strings.Contains(string(sent.Messages[1].Content), `"kind":"axlr_history_checkpoint"`) || string(sent.Messages[len(sent.Messages)-1].Content) != "sigue" {
		t.Fatalf("expected a checkpoint and the current prompt, got %d messages", len(sent.Messages))
	}
}

// An axlr_history page never exceeds what the projection keeps whole for the
// session's model, whatever limit_bytes asked.
func TestHostHistoryPagesWithinTheProjectionBudget(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn(root.Text(strings.Repeat("x", 40000)), turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("listo")); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("recupera", turnTools()); err != nil {
		t.Fatal(err)
	}
	// A 16K window keeps 4,608 bytes per tool result; a page asked at the
	// 32 KiB maximum shrinks to that, not to the maximum's own 16 KiB.
	budget := domain.ContextBudgetForWindow(16384)
	windows := windowsFunc(func(root.ModelID) domain.ContextWindow { return 16384 })
	identity := domain.ToolIdentity{Kind: domain.ToolKindHost, LocalOperation: domain.HostOperationHistory}
	outcome, err := HostToolUseCase{Windows: windows}.Execute(context.Background(), s, identity, hostJSON(t, `{"message_index":0,"limit_bytes":32768}`))
	if err != nil || outcome.IsError {
		t.Fatalf("history page: err=%v outcome=%+v", err, outcome)
	}
	var page struct {
		Text    string `json:"text"`
		HasMore bool   `json:"has_more"`
	}
	if err := json.Unmarshal([]byte(outcome.Content), &page); err != nil {
		t.Fatal(err)
	}
	if !page.HasMore || len(page.Text) >= 32768 || contentJSONBytes(string(outcome.Content)) > budget.ToolResultBytes()-256 {
		t.Fatalf("page of %d bytes (has_more=%v) for a tool result budget of %d", len(page.Text), page.HasMore, budget.ToolResultBytes())
	}
	// The page then survives the projection of the turn it belongs to.
	projector, _ := NewModelContextProjector(budget)
	projection, err := projector.Project([]root.Message{{Role: root.RoleUser, Content: "recupera"}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call-1", Name: "axlr_history", Arguments: hostJSON(t, `{"message_index":0}`)}}}, {Role: root.RoleTool, ToolCallID: "call-1", Content: root.Text(outcome.Content)}})
	if err != nil || string(projection.Messages[2].Content) != string(outcome.Content) {
		t.Fatalf("history page was excerpted by the projection: err=%v", err)
	}
	// Without Windows the 32 KiB maximum stands.
	bounded := len(page.Text)
	outcome, err = HostToolUseCase{}.Execute(context.Background(), s, identity, hostJSON(t, `{"message_index":0,"limit_bytes":32768}`))
	if err != nil || outcome.IsError {
		t.Fatalf("unbounded page: err=%v outcome=%+v", err, outcome)
	}
	if err := json.Unmarshal([]byte(outcome.Content), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Text) <= bounded || contentJSONBytes(string(outcome.Content)) > MaxHistoryReadBytes-256 {
		t.Fatalf("unbounded page of %d bytes against a bounded one of %d", len(page.Text), bounded)
	}
}

// A clipped result reads the same while its turn is open and after it has
// closed, so closing a turn does not rewrite the provider's cached prefix.
func TestClippedResultKeepsItsTextWhenItsTurnCloses(t *testing.T) {
	big := root.Text(`{"rows":"` + strings.Repeat("r", 3*domain.DefaultContextBudget().ToolResultBytes()) + `"}`)
	open := []root.Message{
		{Role: root.RoleUser, Content: "lista"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call-1", Name: "local_exec", Arguments: hostJSON(t, `{}`)}}},
		{Role: root.RoleTool, ToolCallID: "call-1", Content: big},
	}
	projector := NewDefaultModelContextProjector()
	during, err := projector.Project(open)
	if err != nil {
		t.Fatal(err)
	}
	closed := append(append([]root.Message(nil), open...), root.Message{Role: root.RoleAssistant, Content: "hecho"}, root.Message{Role: root.RoleUser, Content: "sigue"})
	after, err := projector.Project(closed)
	if err != nil {
		t.Fatal(err)
	}
	if during.Messages[2].Content != after.Messages[2].Content || !strings.Contains(string(after.Messages[2].Content), `"kind":"axlr_tool_result_excerpt"`) || !strings.Contains(string(after.Messages[2].Content), "message_index: 2") {
		t.Fatalf("excerpt changed when the turn closed:\n%s\n%s", during.Messages[2].Content, after.Messages[2].Content)
	}
}

// A compact ceremony step projects at most 8 KiB per tool result, so its
// axlr_history pages shrink to that as well.
func TestHostHistoryPagesWithinTheCompactBudget(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn(root.Text(strings.Repeat("x", 40000)), turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("listo")); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("recupera", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_delivery", Version: "2.0", Instance: "i", Step: "build", Iteration: 1, Compact: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, compact := compactRun(s); !compact {
		t.Fatal("expected a compact step")
	}
	windows := windowsFunc(func(root.ModelID) domain.ContextWindow { return 0 })
	identity := domain.ToolIdentity{Kind: domain.ToolKindHost, LocalOperation: domain.HostOperationHistory}
	outcome, err := HostToolUseCase{Windows: windows}.Execute(context.Background(), s, identity, hostJSON(t, `{"message_index":0,"limit_bytes":32768}`))
	if err != nil || outcome.IsError {
		t.Fatalf("history page: err=%v outcome=%+v", err, outcome)
	}
	if limit := domain.CompactContextBudget().ToolResultBytes(); contentJSONBytes(string(outcome.Content)) > limit-256 {
		t.Fatalf("page of %d encoded bytes for a compact result budget of %d", contentJSONBytes(string(outcome.Content)), limit)
	}
}
