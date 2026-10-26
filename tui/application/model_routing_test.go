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

// routeFacts is what a fake console knows of each candidate.
type routeFacts map[root.ModelID]domain.RouteCandidate

func (f routeFacts) RouteFacts(_ context.Context, model root.ModelID) domain.RouteCandidate {
	facts := f[model]
	facts.PromptRate, facts.CompletionRate, facts.RatesKnown = 0, 0, false
	return facts
}

func (f routeFacts) CatalogPrice(_ context.Context, model root.ModelID) (float64, float64, bool) {
	facts := f[model]
	return facts.PromptRate, facts.CompletionRate, facts.RatesKnown
}

// memoryModelStats keeps each model's observations in memory.
type memoryModelStats struct {
	histories map[root.ModelID]*domain.ModelHistory
}

func (m *memoryModelStats) ObserveModel(_ context.Context, model root.ModelID, observation domain.ModelObservation) error {
	if m.histories == nil {
		m.histories = map[root.ModelID]*domain.ModelHistory{}
	}
	if m.histories[model] == nil {
		m.histories[model] = &domain.ModelHistory{}
	}
	m.histories[model].Add(observation)
	return nil
}

func (m *memoryModelStats) ModelStats(_ context.Context, models []root.ModelID, now time.Time) (map[root.ModelID]domain.ModelStats, error) {
	out := map[root.ModelID]domain.ModelStats{}
	for _, model := range models {
		if history := m.histories[model]; history != nil {
			out[model] = history.Stats(now)
		}
	}
	return out, nil
}

// gx10Facts is a local model on this host and Claude Haiku on OpenRouter.
var gx10Facts = routeFacts{
	"qwen-local":                 {Local: true, Reachable: true, Cap: 100000},
	"anthropic/claude-haiku-5.5": {Reachable: true, Cap: 100000, PromptRate: 1e-6, CompletionRate: 5e-6, RatesKnown: true},
}

func routedSession(t *testing.T, objective domain.RoutingObjective, allowRemote bool, candidates ...root.ModelID) domain.Session {
	t.Helper()
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	routing, err := domain.NewExecutionRouting(candidates, objective, allowRemote, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRouting(routing); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	return s
}

// modelsAsked answers by model: a model in failing fails before any sign,
// the others answer "done from <model>".
func modelsAsked(asked *[]root.ModelID, failing ...root.ModelID) streamFunc {
	return func(_ context.Context, req root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
		*asked = append(*asked, req.Model)
		for _, model := range failing {
			if req.Model == model {
				return root.CompletionResult{}, errors.New("dial tcp 127.0.0.1:8080: connect: connection refused")
			}
		}
		text := root.Text("done from " + string(req.Model))
		if err := onText(text); err != nil {
			return root.CompletionResult{}, err
		}
		result := assistant(text)
		result.Usage = &root.TokenUsage{PromptTokens: 900, CompletionTokens: 20}
		return result, nil
	}
}

func routedEvents(events []Event) []string {
	var out []string
	for _, event := range events {
		if event.Kind == EventModelRouted {
			line := string(event.Route) + ":" + string(event.Model)
			if event.RouteFrom != "" {
				line += "<" + string(event.RouteFrom)
			}
			out = append(out, line)
		}
	}
	return out
}

// cost sends the request to the local model, says so, and journals and
// observes it under that model.
func TestRoutingSendsTheRequestToTheModelItRanksFirst(t *testing.T) {
	s := routedSession(t, domain.RoutingCost, true, "anthropic/claude-haiku-5.5", "qwen-local")
	var asked []root.ModelID
	receipts, stats, ledgers := &memoryReceipts{}, &memoryModelStats{}, usageLedgers{}
	u := ContinueTurnUseCase{Models: modelsAsked(&asked), Store: &memoryStore{}, Receipts: receipts, Usage: ledgers, Routing: gx10Facts, ModelStats: stats}
	var events []Event
	if err := u.Execute(context.Background(), &s, func(e Event) error { events = append(events, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != "qwen-local" || s.Status() != domain.StatusComplete {
		t.Fatalf("asked %v, status %s", asked, s.Status())
	}
	if got := routedEvents(events); strings.Join(got, ",") != "ranked:qwen-local" {
		t.Fatalf("routed events %v", got)
	}
	if first := receipts.entries[0]; first.Model != "qwen-local" || first.Route != domain.RouteRanked {
		t.Fatalf("receipt %+v", first)
	}
	if observed := stats.histories["qwen-local"]; observed == nil || observed.Stats(time.Now()).Requests != 1 {
		t.Fatalf("stats %+v", stats.histories)
	}
	// The session keeps its own model: only the request went elsewhere.
	if s.Export().Model != "anthropic/claude-haiku-5.5" {
		t.Fatalf("session model %s", s.Export().Model)
	}
}

// A provider error before any sign sends the request again, once, to the
// next candidate: both requests are in the ledger, the receipt and the
// stats, and the events say which model took it and why.
func TestRoutingFallsBackOnceToTheNextCandidate(t *testing.T) {
	s := routedSession(t, domain.RoutingCost, true, "qwen-local", "anthropic/claude-haiku-5.5")
	var asked []root.ModelID
	receipts, stats, ledgers := &memoryReceipts{}, &memoryModelStats{}, usageLedgers{}
	u := ContinueTurnUseCase{Models: modelsAsked(&asked, "qwen-local"), Store: &memoryStore{}, Receipts: receipts, Usage: ledgers, Routing: gx10Facts, ModelStats: stats}
	var events []Event
	if err := u.Execute(context.Background(), &s, func(e Event) error { events = append(events, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(modelStrings(asked), ",") != "qwen-local,anthropic/claude-haiku-5.5" || s.Status() != domain.StatusComplete {
		t.Fatalf("asked %v, status %s", asked, s.Status())
	}
	if got := routedEvents(events); strings.Join(got, ",") != "ranked:qwen-local,fallback:anthropic/claude-haiku-5.5<qwen-local" {
		t.Fatalf("routed events %v", got)
	}
	for _, event := range events {
		if event.Route == domain.RouteFallback && !strings.Contains(event.RouteError, "connection refused") {
			t.Fatalf("the fallback event does not say why: %+v", event)
		}
	}
	var lines []string
	for _, entry := range receipts.entries {
		lines = append(lines, string(entry.Kind)+":"+string(entry.Model)+string(entry.Route)+string(entry.Status))
	}
	if strings.Join(lines, ",") != "request.started:qwen-localranked,request.ended:failed,request.started:anthropic/claude-haiku-5.5fallback,request.ended:completed" {
		t.Fatalf("receipt %v", lines)
	}
	if ledger := ledgers[s.ID()]; ledger.Requests != 2 {
		t.Fatalf("ledger counted %d requests", ledger.Requests)
	}
	if local := stats.histories["qwen-local"].Stats(time.Now()); local.Failures != 1 {
		t.Fatalf("local stats %+v", local)
	}
	if remote := stats.histories["anthropic/claude-haiku-5.5"].Stats(time.Now()); remote.Completed != 1 {
		t.Fatalf("remote stats %+v", remote)
	}
	if !strings.Contains(string(s.Messages()[len(s.Messages())-1].Content), "done from anthropic/claude-haiku-5.5") {
		t.Fatalf("answer %q", s.Messages()[len(s.Messages())-1].Content)
	}
	// Once only: both failing is the second's error, never a third request.
	s = routedSession(t, domain.RoutingQualityFirst, true, "qwen-local", "anthropic/claude-haiku-5.5", "other")
	asked = nil
	u.Routing = routeFacts{"qwen-local": gx10Facts["qwen-local"], "anthropic/claude-haiku-5.5": gx10Facts["anthropic/claude-haiku-5.5"], "other": {Reachable: true}}
	u.Models = modelsAsked(&asked, "qwen-local", "anthropic/claude-haiku-5.5")
	if err := u.Execute(context.Background(), &s, ignoreEvent); err == nil || len(asked) != 2 {
		t.Fatalf("err %v, asked %v", err, asked)
	}
}

// With remote providers forbidden, a failing local model is the end of
// the request: it never reaches the remote candidate.
func TestRoutingWithoutRemoteProvidersNeverLeavesTheHost(t *testing.T) {
	s := routedSession(t, domain.RoutingCost, false, "qwen-local", "anthropic/claude-haiku-5.5")
	var asked []root.ModelID
	receipts := &memoryReceipts{}
	u := ContinueTurnUseCase{Models: modelsAsked(&asked, "qwen-local"), Store: &memoryStore{}, Receipts: receipts, Routing: gx10Facts}
	var events []Event
	err := u.Execute(context.Background(), &s, func(e Event) error { events = append(events, e); return nil })
	var unavailable *RouteUnavailableError
	if !errors.As(err, &unavailable) || strings.Join(modelStrings(asked), ",") != "qwen-local" {
		t.Fatalf("err %v, asked %v", err, asked)
	}
	for _, entry := range receipts.entries {
		if entry.Model == "anthropic/claude-haiku-5.5" {
			t.Fatalf("the receipt lists a remote request: %+v", receipts.entries)
		}
	}
	if got := routedEvents(events); strings.Join(got, ",") != "ranked:qwen-local" {
		t.Fatalf("routed events %v", got)
	}
	// Only remote candidates: nothing is asked at all.
	s = routedSession(t, domain.RoutingCost, false, "anthropic/claude-haiku-5.5")
	asked = nil
	err = u.Execute(context.Background(), &s, ignoreEvent)
	if !errors.As(err, &unavailable) || len(asked) != 0 || !strings.Contains(err.Error(), "does not allow remote providers") {
		t.Fatalf("err %v, asked %v", err, asked)
	}
}

// A failure after the provider showed something is not taken again: the
// person already saw part of the answer.
func TestRoutingDoesNotFallBackOnceTheAnswerStarted(t *testing.T) {
	s := routedSession(t, domain.RoutingCost, true, "qwen-local", "anthropic/claude-haiku-5.5")
	var asked []root.ModelID
	u := ContinueTurnUseCase{Store: &memoryStore{}, Routing: gx10Facts, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
		asked = append(asked, req.Model)
		_ = onText("half an ans")
		return root.CompletionResult{}, errors.New("stream idle timeout")
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err == nil || len(asked) != 1 || s.Status() != domain.StatusInterrupted {
		t.Fatalf("err %v, asked %v, status %s", err, asked, s.Status())
	}
	// Nor is a request the caller cancelled.
	s = routedSession(t, domain.RoutingCost, true, "qwen-local", "anthropic/claude-haiku-5.5")
	asked = nil
	ctx, cancel := context.WithCancel(context.Background())
	u.Models = streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		asked = append(asked, req.Model)
		cancel()
		return root.CompletionResult{}, context.Canceled
	})
	if err := u.Execute(ctx, &s, ignoreEvent); err == nil || len(asked) != 1 {
		t.Fatalf("err %v, asked %v", err, asked)
	}
}

// The fallback is another request: under a budget of one request the
// execution ends at its budget instead.
func TestRoutingFallbackCountsAgainstTheBudget(t *testing.T) {
	s := routedSession(t, domain.RoutingCost, true, "qwen-local", "anthropic/claude-haiku-5.5")
	budget, _ := domain.NewExecutionBudget(0, 1, 0, 0)
	if err := s.SetBudget(budget, time.Now(), domain.UsageLedger{}); err != nil {
		t.Fatal(err)
	}
	var asked []root.ModelID
	u := ContinueTurnUseCase{Models: modelsAsked(&asked, "qwen-local"), Store: &memoryStore{}, Usage: usageLedgers{}, Routing: gx10Facts}
	err := u.Execute(context.Background(), &s, ignoreEvent)
	var exceeded *domain.BudgetExceededError
	if !errors.As(err, &exceeded) || exceeded.Limit != domain.BudgetRequests || len(asked) != 1 {
		t.Fatalf("err %v, asked %v", err, asked)
	}
	if stop := s.BudgetStop(); stop == nil {
		t.Fatal("the session does not say the budget ended it")
	}
}

// A request too large for the local window goes to the remote candidate;
// past every candidate's cap the turn continues in a successor, at the
// largest cap; max_context_tokens narrows every cap.
func TestRoutingRespectsEachCandidatesWindow(t *testing.T) {
	facts := routeFacts{
		"qwen-local":                 {Local: true, Reachable: true, Cap: 60},
		"anthropic/claude-haiku-5.5": {Reachable: true, Cap: 100000, PromptRate: 1e-6, CompletionRate: 5e-6, RatesKnown: true},
	}
	var asked []root.ModelID
	u := ContinueTurnUseCase{Models: modelsAsked(&asked), Store: &memoryStore{}, Routing: facts, Tokens: &byteCounter{}, Successor: &successorStub{}}
	s := routedSession(t, domain.RoutingCost, true, "qwen-local", "anthropic/claude-haiku-5.5")
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil || strings.Join(modelStrings(asked), ",") != "anthropic/claude-haiku-5.5" {
		t.Fatalf("err %v, asked %v", err, asked)
	}
	// Remote forbidden: the local window alone, and past it a successor.
	s = routedSession(t, domain.RoutingCost, false, "qwen-local", "anthropic/claude-haiku-5.5")
	if err := s.CompleteAssistant(assistant("hecho")); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn(root.Text("otra "+strings.Repeat("x", 400)), turnTools()); err != nil {
		t.Fatal(err)
	}
	asked = nil
	err := u.Execute(context.Background(), &s, ignoreEvent)
	capped, ok := IsContextCap(err)
	if !ok || capped.Cap != 60 || len(asked) != 0 {
		t.Fatalf("err %v, asked %v", err, asked)
	}
	// max_context_tokens narrows the remote cap too.
	s = routedSession(t, domain.RoutingCost, true, "qwen-local", "anthropic/claude-haiku-5.5")
	narrow, _ := domain.NewExecutionRouting([]root.ModelID{"qwen-local", "anthropic/claude-haiku-5.5"}, domain.RoutingCost, true, 40, 0)
	if err := s.SetRouting(narrow); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("hecho")); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn(root.Text("otra "+strings.Repeat("x", 400)), turnTools()); err != nil {
		t.Fatal(err)
	}
	if capped, ok := IsContextCap(u.Execute(context.Background(), &s, ignoreEvent)); !ok || capped.Cap != 40 || len(asked) != 0 {
		t.Fatalf("capped %+v, asked %v", capped, asked)
	}
}

// A console that knows nothing of the candidates refuses the request
// rather than sending it to the session's model.
func TestRoutingWithoutFactsRefusesTheRequest(t *testing.T) {
	s := routedSession(t, domain.RoutingCost, true, "qwen-local")
	var asked []root.ModelID
	u := ContinueTurnUseCase{Models: modelsAsked(&asked), Store: &memoryStore{}}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err == nil || len(asked) != 0 {
		t.Fatalf("err %v, asked %v", err, asked)
	}
}

// Every request feeds the model's stats, routed or not; a cancelled one
// says nothing of the model.
func TestEveryRequestFeedsTheModelStats(t *testing.T) {
	stats := &memoryModelStats{}
	var asked []root.ModelID
	u := ContinueTurnUseCase{Models: modelsAsked(&asked), Store: &memoryStore{}, ModelStats: stats}
	s := turnSession(t)
	if err := s.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if history := stats.histories["test/model"]; history == nil || history.Stats(time.Now()).Completed != 1 {
		t.Fatalf("stats %+v", stats.histories)
	}
}

func modelStrings(models []root.ModelID) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		out = append(out, string(model))
	}
	return out
}
