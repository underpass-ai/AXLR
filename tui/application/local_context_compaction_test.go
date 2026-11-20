package application

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/adapters/provider"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestLocalContextCompactsAndContinuesInSameSession(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("server_overflow=%t", retry), func(t *testing.T) {
			session := turnSession(t)
			if err := session.BeginTurn("Keep the original tests", turnTools()); err != nil {
				t.Fatal(err)
			}
			earlierReply := root.Text("Decided to keep parser behavior. " + strings.Repeat("detail ", 5000))
			if err := session.CompleteAssistant(assistant(earlierReply)); err != nil {
				t.Fatal(err)
			}
			if err := session.BeginTurn("Continue with the parser", turnTools()); err != nil {
				t.Fatal(err)
			}
			if err := session.CompleteAssistant(assistant("", call("current-read", "read"))); err != nil {
				t.Fatal(err)
			}
			if err := session.RecordToolOutcome("current-read", domain.DecisionDeny, domain.ToolOutcome{Content: "denied"}); err != nil {
				t.Fatal(err)
			}

			originalID := session.ID()
			windows := &servedWindowRecorder{}
			store, successor := &memoryStore{}, &successorStub{}
			counter := &byteCounter{}
			attempts, notices := 0, 0
			cap := 6000
			if retry {
				cap = 24000
			}
			useCase := ContinueTurnUseCase{
				Store: store, Successor: successor, Tokens: counter, Windows: windows,
				LocalModel: func(root.ModelID) bool { return true },
				ContextCap: func(root.ModelID) int { return cap },
				Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
					attempts++
					tokens := RequestTokens(counter, req.Messages[0], req.Tools, req.Messages[1:])
					if retry && attempts == 1 {
						if req.Messages[2].Content != earlierReply {
							t.Fatal("initial request lost history before reaching the configured cap")
						}
						return root.CompletionResult{}, &provider.ProviderError{StatusCode: 400, Message: fmt.Sprintf("request (%d tokens) exceeds the available context size (8000 tokens), try increasing it", tokens)}
					}
					if len(req.Messages) != 5 {
						t.Fatalf("expected guidance, checkpoint and current turn, got %d messages", len(req.Messages))
					}
					if summary := string(req.Messages[1].Content); !strings.Contains(summary, "Keep the original tests") ||
						!strings.Contains(summary, "Decided to keep parser behavior") {
						t.Fatalf("checkpoint did not carry prior request and decision: %.800s", summary)
					}
					if req.Messages[2].Content != "Continue with the parser" || req.Messages[3].ToolCalls[0].ID != "current-read" || req.Messages[4].ToolCallID != "current-read" {
						t.Fatal("active turn or tool pair changed")
					}
					if tokens > 6000 {
						t.Fatalf("compacted request still over budget: %d tokens", tokens)
					}
					return assistant("continued"), nil
				}),
			}
			err := useCase.Execute(context.Background(), &session, func(event Event) error {
				if event.Kind == EventContextCompacted {
					notices++
				}
				return nil
			})
			wantAttempts := 1
			if retry {
				wantAttempts++
			}
			if err != nil || attempts != wantAttempts || notices != 1 || len(successor.created) != 0 {
				t.Fatalf("err=%v attempts=%d notices=%d successors=%v", err, attempts, notices, successor.created)
			}
			// The window the server reported sizes the launch's later
			// requests (20 November 2026: one refusal per step before).
			if want := map[bool]int{false: 0, true: 8000}[retry]; windows.observed[session.Export().Model] != want || len(windows.observed) > 1 {
				t.Fatalf("served windows observed = %v, want %d", windows.observed, want)
			}
			saved := store.states[len(store.states)-1]
			if session.ID() != originalID || len(saved.Messages) != 6 || saved.Messages[1].Content != earlierReply ||
				saved.Messages[5].Content != "continued" {
				t.Fatal("compaction changed the session or stored transcript")
			}
		})
	}
}

func TestLocalContextReportsActiveRequestThatCannotFit(t *testing.T) {
	session := turnSession(t)
	if err := session.BeginTurn(root.Text(strings.Repeat("x", 24000)), turnTools()); err != nil {
		t.Fatal(err)
	}
	successor := &successorStub{}
	useCase := ContinueTurnUseCase{
		Store: &memoryStore{}, Successor: successor, Tokens: &byteCounter{},
		LocalModel: func(root.ModelID) bool { return true },
		ContextCap: func(root.ModelID) int { return 6000 },
		Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
			t.Fatal("over-budget active request was sent to the model")
			return root.CompletionResult{}, nil
		}),
	}
	err := useCase.Execute(context.Background(), &session, ignoreEvent)
	if err == nil || !strings.Contains(err.Error(), "input budget of 6000") ||
		!strings.Contains(err.Error(), "local_models.context_tokens") || len(successor.created) != 0 {
		t.Fatalf("err=%v successors=%v", err, successor.created)
	}
}

func TestLocalContextCompactsCompletedResultsInActiveTurn(t *testing.T) {
	const cap = 12288
	const resultCount = 12
	session := turnSession(t)
	id := session.ID()
	prompt := root.Text("Continue the report")
	if err := session.BeginTurn(prompt, turnTools()); err != nil {
		t.Fatal(err)
	}
	calls := make([]root.ToolCall, resultCount)
	for i := range calls {
		calls[i] = call(root.ToolCallID(fmt.Sprintf("finished-%d", i)), "read")
	}
	if err := session.CompleteAssistant(assistant("", calls...)); err != nil {
		t.Fatal(err)
	}
	original := root.Text(strings.Repeat("page text ", 470))
	for _, completed := range calls {
		if err := session.RecordToolOutcome(completed.ID, domain.DecisionApprove, domain.ToolOutcome{Content: original}); err != nil {
			t.Fatal(err)
		}
	}
	store, successor, counter := &memoryStore{}, &successorStub{}, &byteCounter{}
	attempts, notices := 0, 0
	useCase := ContinueTurnUseCase{
		Store: store, Successor: successor, Tokens: counter,
		LocalModel: func(root.ModelID) bool { return true },
		ContextCap: func(root.ModelID) int { return cap },
		Windows:    windowsFunc(func(root.ModelID) domain.ContextWindow { return 16384 }),
		Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
			attempts++
			if len(req.Messages) != resultCount+3 || req.Messages[1].Content != prompt {
				t.Fatal("compaction removed messages from the active turn")
			}
			if tokens := RequestTokens(counter, req.Messages[0], req.Tools, req.Messages[1:]); tokens > cap {
				t.Fatalf("projected %d tokens, input budget is %d", tokens, cap)
			}
			for i, projected := range req.Messages[3:] {
				sent := req.Messages[2].ToolCalls[i]
				if sent.ID != calls[i].ID || sent.Name != calls[i].Name || string(sent.Arguments.Bytes()) != string(calls[i].Arguments.Bytes()) || projected.ToolCallID != calls[i].ID {
					t.Fatal("compaction changed a completed call or detached its result")
				}
				if !strings.Contains(string(projected.Content), fmt.Sprintf("axlr_history({message_index: %d,", i+2)) {
					t.Fatalf("result %d has no exact history reference", i)
				}
			}
			return assistant("continued the report"), nil
		}),
	}
	err := useCase.Execute(context.Background(), &session, func(event Event) error {
		if event.Kind == EventContextCompacted {
			notices++
		}
		return nil
	})
	if err != nil || attempts != 1 || notices != 1 || len(successor.created) != 0 || session.ID() != id {
		t.Fatalf("err=%v attempts=%d notices=%d successors=%v", err, attempts, notices, successor.created)
	}
	saved := store.states[len(store.states)-1]
	for i, message := range saved.Messages[2 : resultCount+2] {
		if message.Content != original || message.ToolCallID != calls[i].ID {
			t.Fatalf("original tool result %d changed", i)
		}
	}
	if saved.Messages[resultCount+2].Content != "continued the report" {
		t.Fatal("assistant answer was not saved")
	}
	if err := session.BeginTurn("Check the report", turnTools()); err != nil {
		t.Fatal(err)
	}
	page, err := hostHistory(session.Messages(), hostJSON(t, `{"message_index":2,"limit_bytes":8192}`), MaxHistoryReadBytes)
	if err != nil || page.(map[string]any)["text"] != string(original) {
		t.Fatalf("history did not recover the original result: %v", err)
	}
}

// servedWindowRecorder is a window port that records what a server reported.
type servedWindowRecorder struct{ observed map[root.ModelID]int }

func (r *servedWindowRecorder) ContextWindow(root.ModelID) domain.ContextWindow { return 0 }

func (r *servedWindowRecorder) ContextBudget(root.ModelID) domain.ContextBudget {
	return domain.ContextBudgetForWindow(0)
}

func (r *servedWindowRecorder) ObserveServedWindow(model root.ModelID, tokens int) {
	if r.observed == nil {
		r.observed = map[root.ModelID]int{}
	}
	r.observed[model] = tokens
}

// A local server that words the overflow as OpenAI does (vLLM) is read as
// llama.cpp's is (20 November 2026: only llama.cpp's wording was).
func TestLocalOverflowReadsLlamaAndOpenAIWordings(t *testing.T) {
	for message, want := range map[string][2]int{
		"request (91591 tokens) exceeds the available context size (90112 tokens), try increasing it":                                                                                                       {91591, 90112},
		"This model's maximum context length is 32768 tokens. However, you requested 34001 tokens (33001 in the messages, 1000 in the completion). Please reduce the length of the messages or completion.": {34001, 32768},
		"This model's maximum context length is 128000 tokens. However, your messages resulted in 130500 tokens.":                                                                                           {130500, 128000},
		"maximum context length is 8192 tokens. However, your request has 9000 input tokens.":                                                                                                               {9000, 8192},
	} {
		prompt, window, ok := llamaOverflow(&provider.ProviderError{StatusCode: 400, Message: message})
		if !ok || prompt != want[0] || window != want[1] {
			t.Errorf("%q: prompt=%d window=%d ok=%v, want %v", message, prompt, window, ok, want)
		}
	}
	for _, refused := range []error{
		&provider.ProviderError{StatusCode: 500, Message: "maximum context length is 8192 tokens. However, you requested 9000 tokens"},
		&provider.ProviderError{StatusCode: 400, Message: "invalid tool schema"},
		&provider.ProviderError{StatusCode: 400, Message: "maximum context length is 8192 tokens. However, you requested 100 tokens"},
	} {
		if _, _, ok := llamaOverflow(refused); ok {
			t.Errorf("read %v as an overflow", refused)
		}
	}
}
