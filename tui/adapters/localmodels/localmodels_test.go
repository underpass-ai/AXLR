package localmodels

import (
	"context"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type recordingClient struct{ models []root.ModelID }

func (c *recordingClient) Complete(_ context.Context, req root.CompletionRequest) (root.CompletionResult, error) {
	c.models = append(c.models, req.Model)
	return root.CompletionResult{}, nil
}

func (c *recordingClient) Stream(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
	c.models = append(c.models, req.Model)
	return root.CompletionResult{}, nil
}

func TestRouterSendsLocalModelsToTheirServerUnderTheServerName(t *testing.T) {
	remote, gemma, qwen := &recordingClient{}, &recordingClient{}, &recordingClient{}
	router := Router{Default: remote, Routes: map[root.ModelID]Route{
		"local/gemma": {Client: gemma, Upstream: "gemma-4-31b"},
		"local/qwen":  {Client: qwen},
	}}
	noop := func(root.Text) error { return nil }
	for _, id := range []root.ModelID{"local/gemma", "local/qwen", "openai/gpt-4o"} {
		if _, err := router.Stream(context.Background(), root.CompletionRequest{Model: id}, noop); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := router.Complete(context.Background(), root.CompletionRequest{Model: "local/gemma"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids(gemma.models), ",") != "gemma-4-31b,gemma-4-31b" || strings.Join(ids(qwen.models), ",") != "local/qwen" || strings.Join(ids(remote.models), ",") != "openai/gpt-4o" {
		t.Fatalf("routed: gemma=%v qwen=%v remote=%v", gemma.models, qwen.models, remote.models)
	}
	_, err := Router{Routes: router.Routes}.Stream(context.Background(), root.CompletionRequest{Model: "openai/gpt-4o"}, noop)
	if err == nil || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("remote model without key: %v", err)
	}
}

func ids(models []root.ModelID) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = string(m)
	}
	return out
}

type catalogFunc func(context.Context) ([]domain.AvailableModel, error)

func (f catalogFunc) List(ctx context.Context) ([]domain.AvailableModel, error) { return f(ctx) }

func TestCatalogListsLocalModelsFirstAndSurvivesARemoteFailure(t *testing.T) {
	local := []domain.AvailableModel{{ID: "local/qwen", Name: "Qwen", SupportsTools: true, TextOutput: true}}
	remote := []domain.AvailableModel{{ID: "openai/gpt-4o", Name: "GPT-4o", SupportsTools: true, TextOutput: true}}
	got, err := Catalog{Local: local, Remote: catalogFunc(func(context.Context) ([]domain.AvailableModel, error) { return remote, nil })}.List(context.Background())
	if err != nil || len(got) != 2 || got[0].ID != "local/qwen" || got[1].ID != "openai/gpt-4o" {
		t.Fatalf("merged = %+v, %v", got, err)
	}
	failing := catalogFunc(func(context.Context) ([]domain.AvailableModel, error) { return nil, errors.New("offline") })
	if got, err := (Catalog{Local: local, Remote: failing}).List(context.Background()); err != nil || len(got) != 1 {
		t.Fatalf("remote failure hid local models: %+v, %v", got, err)
	}
	if _, err := (Catalog{Remote: failing}).List(context.Background()); err == nil {
		t.Fatal("remote failure without local models was swallowed")
	}
	if got, err := (Catalog{Local: local}).List(context.Background()); err != nil || len(got) != 1 {
		t.Fatalf("local only = %+v, %v", got, err)
	}
}

func TestWindowsApplyTheCap(t *testing.T) {
	w := Windows{Local: map[root.ModelID]domain.ContextWindow{"local/qwen": 262144, "local/small": 16384}, Cap: 65536}
	for id, want := range map[root.ModelID]domain.ContextWindow{"local/qwen": 65536, "local/small": 16384, "openai/gpt-4o": 65536} {
		if got := w.ContextWindow(id); got != want {
			t.Fatalf("%s = %d, want %d", id, got, want)
		}
	}
	uncapped := Windows{Local: w.Local}
	if uncapped.ContextWindow("local/qwen") != 262144 || uncapped.ContextWindow("openai/gpt-4o") != 0 {
		t.Fatal("uncapped windows changed")
	}
}

type wholeReplyClient struct{ streamed, completed int }

func (c *wholeReplyClient) Complete(context.Context, root.CompletionRequest) (root.CompletionResult, error) {
	c.completed++
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "whole"}}, nil
}

func (c *wholeReplyClient) Stream(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
	c.streamed++
	return root.CompletionResult{}, nil
}

func TestRouterAsksForWholeRepliesWhenStreamingIsOff(t *testing.T) {
	client := &wholeReplyClient{}
	router := Router{Routes: map[root.ModelID]Route{"local/gemma": {Client: client, NoStream: true}}}
	var text []root.Text
	result, err := router.Stream(context.Background(), root.CompletionRequest{Model: "local/gemma"}, func(delta root.Text) error { text = append(text, delta); return nil })
	if err != nil || client.completed != 1 || client.streamed != 0 || len(text) != 1 || text[0] != "whole" || result.Message.Content != "whole" {
		t.Fatalf("completed=%d streamed=%d text=%v err=%v", client.completed, client.streamed, text, err)
	}
}

func TestWindowsBudgetLocalModelsByWindowAndRemoteOnesByPrompt(t *testing.T) {
	w := Windows{Local: map[root.ModelID]domain.ContextWindow{"local/qwen": 65536}}
	if got := w.ContextBudget("local/qwen"); got != domain.ContextBudgetForWindow(65536) {
		t.Fatalf("local model budget = %+v", got)
	}
	if got := w.ContextBudget("anthropic/claude-haiku-5.5"); got != domain.ContextBudgetForPrompt(domain.DefaultPromptTokens) || got == domain.DefaultContextBudget() {
		t.Fatalf("remote model budget = %+v", got)
	}
	w.Prompt = 96000
	if got := w.ContextBudget("anthropic/claude-haiku-5.5"); got != domain.ContextBudgetForPrompt(96000) {
		t.Fatalf("remote model budget with prompt_tokens = %+v", got)
	}
	// The cap still bounds a remote model, and the prompt budget still
	// bounds a cap that holds more.
	w.Cap = 32768
	if got := w.ContextBudget("anthropic/claude-haiku-5.5"); got != domain.ContextBudgetForPrompt(96000).Smaller(domain.ContextBudgetForWindow(32768)) || got.MaximumBytes() != domain.ContextBudgetForWindow(32768).MaximumBytes() {
		t.Fatalf("capped remote model budget = %+v", got)
	}
	w.Cap = 1 << 20
	if got := w.ContextBudget("anthropic/claude-haiku-5.5"); got != domain.ContextBudgetForPrompt(96000) {
		t.Fatalf("remote model under a large cap = %+v", got)
	}
}

type calibrated map[root.ModelID]int

func (c calibrated) BytesPerToken(id root.ModelID) (int, bool) {
	hundredths, ok := c[id]
	return hundredths, ok
}

func TestWindowsBudgetARemoteModelAtItsMeasuredBytesPerToken(t *testing.T) {
	w := Windows{Local: map[root.ModelID]domain.ContextWindow{"local/qwen": 65536}, Calibration: calibrated{"z-ai/glm-5.3-flash": 420, "local/qwen": 420}}
	if got := w.ContextBudget("z-ai/glm-5.3-flash"); got != domain.ContextBudgetForPromptAt(domain.DefaultPromptTokens, 420) || got.MaximumBytes() <= domain.ContextBudgetForPrompt(domain.DefaultPromptTokens).MaximumBytes() {
		t.Fatalf("calibrated remote budget = %+v", got)
	}
	if got := w.ContextBudget("anthropic/claude-haiku-5.5"); got != domain.ContextBudgetForPrompt(domain.DefaultPromptTokens) {
		t.Fatalf("uncalibrated remote budget = %+v", got)
	}
	// A local model's server holds its window: its budget is the window's.
	if got := w.ContextBudget("local/qwen"); got != domain.ContextBudgetForWindow(65536) {
		t.Fatalf("local budget = %+v", got)
	}
	w.Cap = 32768
	if got := w.ContextBudget("z-ai/glm-5.3-flash"); got.MaximumBytes() != domain.ContextBudgetForWindow(32768).MaximumBytes() {
		t.Fatalf("capped calibrated budget = %+v", got)
	}
}
