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
			store, successor := &memoryStore{}, &successorStub{}
			counter := &byteCounter{}
			attempts, notices := 0, 0
			cap := 6000
			if retry {
				cap = 24000
			}
			useCase := ContinueTurnUseCase{
				Store: store, Successor: successor, Tokens: counter,
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
