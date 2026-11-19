package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// passingFailure stands for the provider adapters' errors that say whether
// asking again may succeed.
type passingFailure struct{ transient bool }

func (e passingFailure) Error() string {
	return "model endpoint 127.0.0.1:8790 provider_failure (HTTP 502)"
}
func (e passingFailure) Transient() bool { return e.transient }

func plainSession(t *testing.T) domain.Session {
	t.Helper()
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), "codex/gpt-6.1-sol")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	return s
}

// failingFirst fails the first failures requests with err, then answers.
func failingFirst(asked *int, failures int, err error) streamFunc {
	return func(_ context.Context, req root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
		*asked++
		if *asked <= failures {
			return root.CompletionResult{}, err
		}
		text := root.Text("done")
		if err := onText(text); err != nil {
			return root.CompletionResult{}, err
		}
		return assistant(text), nil
	}
}

var quickRetries = []time.Duration{time.Millisecond, time.Millisecond}

// A failure that may pass, before anything was shown, is asked again of
// the same model; each attempt is in the receipt and the ledger.
func TestATransientFailureIsAskedAgainOfTheSameModel(t *testing.T) {
	s := plainSession(t)
	asked := 0
	receipts, ledgers := &memoryReceipts{}, usageLedgers{}
	u := ContinueTurnUseCase{Models: failingFirst(&asked, 2, passingFailure{transient: true}), Store: &memoryStore{}, Receipts: receipts, Usage: ledgers, ModelRetryWaits: quickRetries}
	var events []Event
	if err := u.Execute(context.Background(), &s, func(e Event) error { events = append(events, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if asked != 3 || s.Status() != domain.StatusComplete {
		t.Fatalf("asked %d, status %s", asked, s.Status())
	}
	if got := routedEvents(events); strings.Join(got, ",") != "retry:codex/gpt-6.1-sol<codex/gpt-6.1-sol,retry:codex/gpt-6.1-sol<codex/gpt-6.1-sol" {
		t.Fatalf("routed events %v", got)
	}
	var lines []string
	for _, entry := range receipts.entries {
		lines = append(lines, string(entry.Kind)+":"+string(entry.Route)+string(entry.Status))
	}
	if strings.Join(lines, ",") != "request.started:,request.ended:failed,request.started:retry,request.ended:failed,request.started:retry,request.ended:completed" {
		t.Fatalf("receipt %v", lines)
	}
	if ledger := ledgers[s.ID()]; ledger.Requests != 3 {
		t.Fatalf("ledger counted %d requests", ledger.Requests)
	}
}

// Past the last wait the failure ends the turn: three requests, no more.
func TestTransientRetriesStopAfterTheLastWait(t *testing.T) {
	s := plainSession(t)
	asked := 0
	u := ContinueTurnUseCase{Models: failingFirst(&asked, 10, passingFailure{transient: true}), Store: &memoryStore{}, ModelRetryWaits: quickRetries}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err == nil || asked != 3 || s.Status() != domain.StatusInterrupted {
		t.Fatalf("err %v, asked %d, status %s", err, asked, s.Status())
	}
	// An empty list never retries.
	s, asked = plainSession(t), 0
	u.ModelRetryWaits = []time.Duration{}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err == nil || asked != 1 {
		t.Fatalf("err %v, asked %d", err, asked)
	}
}

// Not asked again: an error that will not pass, an unknown error, one
// after part of the answer was shown, or one the caller cancelled.
func TestOnlyATransientFailureBeforeAnythingShownIsRetried(t *testing.T) {
	for name, models := range map[string]func(*int) streamFunc{
		"refused": func(asked *int) streamFunc { return failingFirst(asked, 1, passingFailure{transient: false}) },
		"unknown": func(asked *int) streamFunc { return failingFirst(asked, 1, errors.New("stream idle timeout")) },
		"answered": func(asked *int) streamFunc {
			return func(_ context.Context, _ root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
				*asked++
				_ = onText("half an ans")
				return root.CompletionResult{}, passingFailure{transient: true}
			}
		},
	} {
		s := plainSession(t)
		asked := 0
		u := ContinueTurnUseCase{Models: models(&asked), Store: &memoryStore{}, ModelRetryWaits: quickRetries}
		if err := u.Execute(context.Background(), &s, ignoreEvent); err == nil || asked != 1 {
			t.Fatalf("%s: err %v, asked %d", name, err, asked)
		}
	}
	s := plainSession(t)
	asked := 0
	ctx, cancel := context.WithCancel(context.Background())
	u := ContinueTurnUseCase{Store: &memoryStore{}, ModelRetryWaits: []time.Duration{time.Hour}, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		asked++
		cancel()
		return root.CompletionResult{}, passingFailure{transient: true}
	})}
	if err := u.Execute(ctx, &s, ignoreEvent); err == nil || asked != 1 {
		t.Fatalf("cancelled: err %v, asked %d", err, asked)
	}
}

// A retry is another request: under a budget of one request the execution
// ends at its budget instead.
func TestARetryCountsAgainstTheBudget(t *testing.T) {
	s := plainSession(t)
	budget, _ := domain.NewExecutionBudget(0, 1, 0, 0)
	if err := s.SetBudget(budget, time.Now(), domain.UsageLedger{}); err != nil {
		t.Fatal(err)
	}
	asked := 0
	u := ContinueTurnUseCase{Models: failingFirst(&asked, 1, passingFailure{transient: true}), Store: &memoryStore{}, Usage: usageLedgers{}, ModelRetryWaits: quickRetries}
	err := u.Execute(context.Background(), &s, ignoreEvent)
	var exceeded *domain.BudgetExceededError
	if !errors.As(err, &exceeded) || exceeded.Limit != domain.BudgetRequests || asked != 1 {
		t.Fatalf("err %v, asked %d", err, asked)
	}
}

// A routing with another candidate falls back to it, not to the same
// model again.
func TestARoutingFallsBackInsteadOfRetrying(t *testing.T) {
	s := routedSession(t, domain.RoutingCost, true, "qwen-local", "anthropic/claude-haiku-5.5")
	var asked []root.ModelID
	u := ContinueTurnUseCase{Store: &memoryStore{}, Routing: gx10Facts, ModelRetryWaits: quickRetries, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
		asked = append(asked, req.Model)
		if req.Model == "qwen-local" {
			return root.CompletionResult{}, passingFailure{transient: true}
		}
		_ = onText("done")
		return assistant("done"), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil || strings.Join(modelStrings(asked), ",") != "qwen-local,anthropic/claude-haiku-5.5" {
		t.Fatalf("err %v, asked %v", err, asked)
	}
}
