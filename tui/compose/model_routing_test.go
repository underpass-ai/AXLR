package compose

import (
	"context"
	"errors"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestOnThisHostIsTheLoopbackOnly(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://127.0.0.1:8080/v1":           true,
		"http://localhost:8090/v1":           true,
		"http://LOCALHOST.:8090/v1":          true,
		"http://models.localhost/v1":         true,
		"http://[::1]:8082/v1":               true,
		"http://127.0.0.53:8080":             true,
		"http://192.168.1.56:8000/v1":        false,
		"https://api.anthropic.com/v1":       false,
		"http://vllm.lab-llm.svc:8000/v1":    false,
		"http://localhost.example.com:80/v1": false,
		"not a url\x7f":                      false,
	} {
		if got := OnThisHost(raw); got != want {
			t.Errorf("%q: %v, want %v", raw, got, want)
		}
	}
}

type pricedCatalog struct {
	calls int
	err   error
}

func (c *pricedCatalog) List(context.Context) ([]domain.AvailableModel, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	prompt, _ := domain.NewModelRate("0.000001")
	completion, _ := domain.NewModelRate("0.000005")
	return []domain.AvailableModel{{ID: "anthropic/claude-haiku-5.5", Name: "Haiku", PromptRate: prompt, CompletionRate: completion}}, nil
}

func TestModelRoutingKnowsWhereEachCandidateRuns(t *testing.T) {
	settings := storage.UserSettings{LocalModels: []storage.LocalModel{
		{ID: "qwen-local", URL: "http://127.0.0.1:8090/v1", ContextTokens: 32768},
		{ID: "wrx80", URL: "http://192.168.1.56:8000/v1", ContextTokens: 65536},
	}}
	catalog := &pricedCatalog{}
	routing := NewModelRouting(NewLocalModels(settings, func(string) string { return "" }), true, catalog)
	ctx := context.Background()
	local := routing.RouteFacts(ctx, "qwen-local")
	if !local.Local || !local.Reachable || local.Cap != 32768/4*3 {
		t.Fatalf("local facts %+v", local)
	}
	if lan := routing.RouteFacts(ctx, "wrx80"); lan.Local || !lan.Reachable {
		t.Fatalf("a server elsewhere on the network is off this host: %+v", lan)
	}
	if remote := routing.RouteFacts(ctx, "anthropic/claude-haiku-5.5"); remote.Local || !remote.Reachable {
		t.Fatalf("remote facts %+v", remote)
	}
	prompt, completion, known := routing.CatalogPrice(ctx, "anthropic/claude-haiku-5.5")
	if !known || prompt != 1e-6 || completion != 5e-6 {
		t.Fatalf("price %v %v %v", prompt, completion, known)
	}
	if _, _, known := routing.CatalogPrice(ctx, "other/model"); known || catalog.calls != 1 {
		t.Fatalf("an unlisted model is priced, or the catalog was read %d times", catalog.calls)
	}
	if _, _, known := routing.CatalogPrice(ctx, "qwen-local"); known {
		t.Fatal("a local_models entry has no catalog price")
	}
	// Without an OpenRouter key a remote model is unreachable and nothing
	// reads the catalog.
	offline := NewModelRouting(NewLocalModels(settings, func(string) string { return "" }), false, catalog)
	if remote := offline.RouteFacts(ctx, "anthropic/claude-haiku-5.5"); remote.Reachable {
		t.Fatalf("remote without a key %+v", remote)
	}
	if _, _, known := offline.CatalogPrice(ctx, "anthropic/claude-haiku-5.5"); known {
		t.Fatal("priced without a key")
	}
	failing := &pricedCatalog{err: errors.New("offline")}
	broken := NewModelRouting(NewLocalModels(settings, func(string) string { return "" }), true, failing)
	for i := 0; i < 2; i++ {
		if _, _, known := broken.CatalogPrice(ctx, root.ModelID("anthropic/claude-haiku-5.5")); known {
			t.Fatal("priced from a catalog that failed")
		}
	}
	if failing.calls != 1 {
		t.Fatalf("a failed catalog was read %d times", failing.calls)
	}
}
