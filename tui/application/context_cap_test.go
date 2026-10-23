package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// byteCounter counts a token per four bytes, like a rough tokenizer.
type byteCounter struct{ calls int }

func (c *byteCounter) Count(text string) int { c.calls++; return (len(text) + 3) / 4 }

type successorStub struct {
	created []domain.SessionID
	err     error
}

func (s *successorStub) Continue(_ context.Context, predecessor domain.Session) (domain.Session, error) {
	if s.err != nil {
		return domain.Session{}, s.err
	}
	successor, err := domain.NewSession("fedcba9876543210fedcba9876543210", predecessor.Export().Workspace, predecessor.Export().Model)
	if err != nil {
		return domain.Session{}, err
	}
	_ = successor.SetInstance(domain.SessionInstance{Predecessor: predecessor.ID()})
	s.created = append(s.created, successor.ID())
	return successor, nil
}

func TestRequestTokensCountsGuidanceToolsAndMessages(t *testing.T) {
	counter := &byteCounter{}
	schema, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
	tools := []root.ToolDefinition{{Name: "read", Description: "reads", Parameters: schema}}
	messages := []root.Message{{Role: root.RoleUser, Content: root.Text(strings.Repeat("a", 400))}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "c", Name: "read", Arguments: schema}}}}
	got := RequestTokens(counter, root.Message{Role: root.RoleSystem, Content: root.Text(strings.Repeat("g", 40))}, tools, messages)
	// guidance 10+4, tool ~ (json ~64 bytes → 16)+4, user 100+4, assistant 4 + call (4 + 1 + 4).
	if got < 140 || got > 170 || RequestTokens(nil, root.Message{}, nil, nil) != 0 {
		t.Fatalf("tokens %d", got)
	}
	budget := cappedBudget(domain.CompactContextBudget())
	if budget.MaximumBytes() != domain.DefaultContextBudget().MaximumBytes() || budget.ToolResultBytes() != domain.CompactContextBudget().ToolResultBytes() {
		t.Fatalf("capped budget %+v", budget)
	}
}

// A request past the cap stops the turn, closes the instance and continues
// the prompt in the successor; the error names it.
func TestContinueTurnContinuesInASuccessorPastTheCap(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	engine := &fakeEngine{}
	ceremony, _, journal, _ := sessionCeremonyFor(t, engine)
	ceremony.Store = store
	successor := &successorStub{}
	asked := false
	u := ContinueTurnUseCase{Store: store, Session: ceremony, Tokens: &byteCounter{}, ContextCap: func(root.ModelID) int { return 50 }, Successor: successor, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		asked = true
		return assistant("no"), nil
	})}
	// A prompt that exceeds the cap on its own is refused, not continued.
	if err := s.BeginTurn(root.Text("explica "+strings.Repeat("x", 600)), turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, func(Event) error { return nil }); err == nil || !strings.Contains(err.Error(), "prompt alone") || len(successor.created) != 0 || s.Status() != domain.StatusInterrupted {
		t.Fatalf("%v %s", err, s.Status())
	}
	s = turnSession(t)
	if err := s.BeginTurn("primero", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("hecho")); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn(root.Text("explica "+strings.Repeat("x", 600)), turnTools()); err != nil {
		t.Fatal(err)
	}
	ceremony.BeginTurn(context.Background(), &s)
	err := u.Execute(context.Background(), &s, func(Event) error { return nil })
	capped, ok := IsContextCap(err)
	if !ok || asked || capped.Cap != 50 || capped.Tokens <= 50 || capped.Predecessor != s.ID() || capped.Successor.ID() != "fedcba9876543210fedcba9876543210" || !strings.HasPrefix(capped.Prompt, "explica ") {
		t.Fatalf("%v asked=%v", err, asked)
	}
	if s.Status() != domain.StatusInterrupted {
		t.Fatalf("status %s", s.Status())
	}
	instance, _ := s.Instance()
	if !instance.Closed || !strings.Contains(strings.Join(engine.calls, ";"), "transition closed") {
		t.Fatalf("instance %+v calls %v", instance, engine.calls)
	}
	if len(journal.entries) != 1 || !strings.Contains(journal.entries[0].Summary, "continued at") {
		t.Fatalf("entries %+v", journal.entries)
	}
	if next, _ := capped.Successor.Instance(); next.Predecessor != s.ID() {
		t.Fatalf("successor %+v", next)
	}
	// Under the cap the request goes out.
	under := turnSession(t)
	if err := under.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	u.ContextCap = func(root.ModelID) int { return 100000 }
	if err := u.Execute(context.Background(), &under, func(Event) error { return nil }); err != nil || !asked {
		t.Fatalf("%v asked=%v", err, asked)
	}
	// A successor that cannot open leaves a plain error.
	successor.err = errors.New("disk full")
	over := turnSession(t)
	if err := over.BeginTurn("primero", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := over.CompleteAssistant(assistant("hecho")); err != nil {
		t.Fatal(err)
	}
	if err := over.BeginTurn(root.Text(strings.Repeat("y", 600)), turnTools()); err != nil {
		t.Fatal(err)
	}
	u.ContextCap = func(root.ModelID) int { return 50 }
	failed := u.Execute(context.Background(), &over, func(Event) error { return nil })
	if failed == nil || !strings.Contains(failed.Error(), "disk full") {
		t.Fatalf("%v", failed)
	}
	if _, ok := IsContextCap(failed); ok {
		t.Fatal("a failed successor is not a continuation")
	}
}

// A successor's first recall wakes the predecessor's about, and its
// instance names the predecessor's.
func TestSuccessorRecallsThePredecessor(t *testing.T) {
	engine := &fakeEngine{}
	recall := &recallStub{texts: map[string]string{"ws:0123456789abcdef0123456789abcdef": "- ws:…:entry:turn:x (turn): Turn 3: 1 file written (a.go)."}}
	c := recallCeremony(t, recall)
	c.Engine = engine
	s, err := domain.NewSession("fedcba9876543210fedcba9876543210", domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetInstance(domain.SessionInstance{Predecessor: "0123456789abcdef0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}
	note := c.Recall(context.Background(), &s, "sigue")
	if !strings.Contains(note, "Memory of ws:0123456789abcdef0123456789abcdef:") || len(recall.requests) != 3 {
		t.Fatalf("note %q requests %d", note, len(recall.requests))
	}
	if err := s.BeginTurn("sigue", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if !strings.Contains(strings.Join(engine.calls, ";"), "start axlr_session 1.0") || engine.inputs["predecessor"] != "axlr-session-0123456789abcdef0123456789abcdef" {
		t.Fatalf("calls %v inputs %v", engine.calls, engine.inputs)
	}
}

func TestTokenScaleFollowsTheProvidersCount(t *testing.T) {
	var scale TokenScale
	if scale.Factor("m") != 1 || scale.Estimate("m", 100) != 100 {
		t.Fatal("a fresh scale is one")
	}
	scale.Observe("m", 100, 300)
	if f := scale.Factor("m"); f != 3 {
		t.Fatalf("factor %v", f)
	}
	scale.Observe("m", 100, 100)
	if f := scale.Factor("m"); f != 2 {
		t.Fatalf("averaged factor %v", f)
	}
	scale.Observe("m", 100, 900)
	if f := scale.Factor("m"); f != 3 {
		t.Fatalf("clamped factor %v", f)
	}
	scale.Observe("m", 100, 10)
	if f := scale.Factor("m"); f != 2 {
		t.Fatalf("floor factor %v", f)
	}
	scale.Observe("other", 0, 10)
	if scale.Factor("other") != 1 || (*TokenScale)(nil).Estimate("m", 7) != 7 {
		t.Fatal("empty observations and a nil scale")
	}
}

// Only the current turn's reasoning goes back, and it is counted at its
// measured size, not tokenized as text (23 October 2026).
func TestReasoningGoesBackForTheCurrentTurnAtItsSize(t *testing.T) {
	signature := strings.Repeat("A", 90_000)
	reasoning, err := root.NewJSONValue([]byte(`[{"type":"reasoning.text","text":"plan the page","signature":"` + signature + `","index":0}]`))
	if err != nil {
		t.Fatal(err)
	}
	messages := []root.Message{
		{Role: root.RoleUser, Content: "first"},
		{Role: root.RoleAssistant, Content: "done", Reasoning: reasoning},
		{Role: root.RoleUser, Content: "second"},
		{Role: root.RoleAssistant, Content: "reading", Reasoning: reasoning},
	}
	sent := currentTurnReasoning(messages)
	if sent[1].HasReasoning() || !sent[3].HasReasoning() || !messages[1].HasReasoning() {
		t.Fatal("the earlier turn's reasoning went back, the current one's did not, or the session's copy changed")
	}
	// "plan the page" is 13 bytes: 4 tokens at four bytes per token.
	if got := reasoningTokens(&byteCounter{}, reasoning); got != 4+30_000 {
		t.Fatalf("reasoning counted as %d tokens", got)
	}
}

// The native adapters' reasoning counts the same way: Anthropic's thinking
// text as text and its signature at its measured size, OpenAI's summary
// parts as text and its encrypted content at its measured size.
func TestReasoningOfTheNativeAdaptersIsCounted(t *testing.T) {
	opaque := strings.Repeat("A", 3_000)
	anthropic, err := root.NewJSONValue([]byte(`[{"type":"thinking","thinking":"plan the page","signature":"` + opaque + `"},{"type":"redacted_thinking","data":"` + opaque + `"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reasoningTokens(&byteCounter{}, anthropic); got != 4+1_000+1_000 {
		t.Fatalf("Anthropic reasoning counted as %d tokens", got)
	}
	openai, err := root.NewJSONValue([]byte(`[{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"plan the page"},{"type":"summary_text","text":"plan the page"}],"encrypted_content":"` + opaque + `"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if got := reasoningTokens(&byteCounter{}, openai); got != 4+4+1_000 {
		t.Fatalf("OpenAI reasoning counted as %d tokens", got)
	}
}
