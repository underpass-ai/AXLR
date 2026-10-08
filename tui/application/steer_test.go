package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type testSteer struct{ text root.Text }

func (q *testSteer) Take() (root.Text, bool) {
	text := q.text
	q.text = ""
	return text, text != ""
}
func (q *testSteer) Restore(text root.Text) { q.text = text + q.text }

// afterToolStep leaves a turn streaming right after one tool result.
func afterToolStep(t *testing.T) (domain.Session, ContinueTurnUseCase) {
	t.Helper()
	s := turnSession(t)
	if err := s.BeginTurn("first request", turnTools()); err != nil {
		t.Fatal(err)
	}
	u := ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("", call("call-1", "read")), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordToolOutcome("call-1", domain.DecisionAutoApprove, domain.ToolOutcome{Content: "file text"}); err != nil {
		t.Fatal(err)
	}
	if s.Status() != domain.StatusStreaming {
		t.Fatalf("status after tool step = %s", s.Status())
	}
	return s, u
}

func TestQueuedMessageJoinsTheTurnAfterItsToolStep(t *testing.T) {
	s, u := afterToolStep(t)
	trace := &diagnosticCollector{}
	u.Diagnostics = trace
	queue := &testSteer{text: "second request"}
	var request root.CompletionRequest
	u.Models = streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		request = r
		return assistant("both answered"), nil
	})
	var kinds []EventKind
	if err := u.Execute(WithSteer(context.Background(), queue), &s, func(e Event) error {
		kinds = append(kinds, e.Kind)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if queue.text != "" || !trace.contains(DiagnosticSteerApplied) {
		t.Fatalf("queue %q, steer recorded %v", queue.text, trace.contains(DiagnosticSteerApplied))
	}
	messages := s.Messages()
	if len(messages) != 5 || messages[3].Role != root.RoleUser || messages[3].Content != "second request" || messages[4].Content != "both answered" {
		t.Fatalf("transcript: %+v", messages)
	}
	last := request.Messages[len(request.Messages)-1]
	if last.Role != root.RoleUser || !strings.HasPrefix(string(last.Content), "second request") || !strings.Contains(string(last.Content), "previous request") {
		t.Fatalf("model did not read the queued message as sent mid-turn: %+v", last)
	}
	if len(kinds) == 0 || kinds[0] != EventSession {
		t.Fatalf("the console did not see the message join before the request: %v", kinds)
	}
	if s.Export().TurnCallCount != 0 {
		t.Fatal("the queued message did not restart the call budget")
	}
	if _, err := domain.RestoreSession(s.Export()); err != nil {
		t.Fatalf("a steered session does not restore: %v", err)
	}
}

func TestQueuedMessageWaitsWhenTheTurnHasNoToolStep(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("first request", turnTools()); err != nil {
		t.Fatal(err)
	}
	queue := &testSteer{text: "second request"}
	u := ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		return assistant("answered"), nil
	})}
	if err := u.Execute(WithSteer(context.Background(), queue), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if queue.text != "second request" || len(s.Messages()) != 2 {
		t.Fatalf("queue %q transcript %+v", queue.text, s.Messages())
	}
}

func TestQueuedMessageNeverSteersACeremony(t *testing.T) {
	s, u := afterToolStep(t)
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_incident", Version: "1.0", Instance: "axlr-i", Step: "present", Iteration: 1, Fence: "f1"}); err != nil {
		t.Fatal(err)
	}
	queue := &testSteer{text: "second request"}
	steered, err := applySteer(WithSteer(context.Background(), queue), &s, u.Store, nil)
	if err != nil || steered || queue.text != "second request" {
		t.Fatalf("steered %v err %v queue %q", steered, err, queue.text)
	}
}

func TestQueuedMessageReturnsToTheQueueWhenItCannotBeSaved(t *testing.T) {
	s, _ := afterToolStep(t)
	queue := &testSteer{text: "second request"}
	steered, err := applySteer(WithSteer(context.Background(), queue), &s, &memoryStore{err: errors.New("disk full")}, nil)
	if err == nil || steered || queue.text != "second request" || len(s.Messages()) != 3 {
		t.Fatalf("steered %v err %v queue %q", steered, err, queue.text)
	}
}

func TestMarkSteeredLeavesHostNotesAndNewTurnsAlone(t *testing.T) {
	messages := []root.Message{
		{Role: root.RoleUser, Content: "first"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{call("c", "read")}},
		{Role: root.RoleTool, ToolCallID: "c", Content: "out"},
		{Role: root.RoleUser, Content: "[AXLR] step reminder"},
		{Role: root.RoleAssistant, Content: "done"},
		{Role: root.RoleUser, Content: "next"},
	}
	marked := markSteered(messages)
	for i := range messages {
		if marked[i].Content != messages[i].Content {
			t.Fatalf("message %d marked: %q", i, marked[i].Content)
		}
	}
}

func TestCancelledStreamReportsTheCancellationOnce(t *testing.T) {
	s, u := afterToolStep(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := u.Execute(ctx, &s, func(Event) error { return ctx.Err() })
	if err == nil || err.Error() != context.Canceled.Error() {
		t.Fatalf("err = %q", err)
	}
}
