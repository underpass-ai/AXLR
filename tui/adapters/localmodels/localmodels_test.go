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
