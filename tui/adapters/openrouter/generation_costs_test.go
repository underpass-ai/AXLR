package openrouter

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// generationStats is the shape of OpenRouter's generation stats as its
// API reference documents them; the values are made up.
const generationStats = `{"data":{"id":"gen-1","total_cost":0.0123,"cache_discount":null,"upstream_inference_cost":0.0123,"created_at":"2026-11-02T10:00:00Z","model":"anthropic/claude-haiku-5.5","origin":"","usage":0.0123,"is_byok":false,"streamed":true,"cancelled":true,"provider_name":"Anthropic","latency":800,"generation_time":9000,"finish_reason":null,"native_finish_reason":null,"tokens_prompt":9500,"tokens_completion":610,"native_tokens_prompt":9800,"native_tokens_completion":600,"native_tokens_reasoning":0}}`

func generationClient(t *testing.T, status int, body string) (GenerationCosts, *int) {
	t.Helper()
	requests := new(int)
	client := &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		*requests++
		if req.Method != http.MethodGet || req.URL.String() != "https://openrouter.ai/api/v1/generation?id=gen-1%26x" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("Authorization = %q", got)
		}
		return catalogResponse(status, body), nil
	})}
	return GenerationCosts{APIKey: "test-secret", HTTPClient: client}, requests
}

// The cost and the provider's own token counts are read from the stats;
// OpenRouter's normalized counts stand in when the native ones are absent.
func TestGenerationCostReadsTheCostAndTokens(t *testing.T) {
	c, requests := generationClient(t, http.StatusOK, generationStats)
	got, err := c.GenerationCost(context.Background(), "gen-1&x")
	if err != nil || *requests != 1 || got != (domain.GenerationCost{Cost: 0.0123, PromptTokens: 9800, CompletionTokens: 600, TokensKnown: true}) {
		t.Fatalf("cost %+v, %v", got, err)
	}
	c, _ = generationClient(t, http.StatusOK, `{"data":{"total_cost":0.5,"tokens_prompt":12.0,"tokens_completion":3,"native_tokens_prompt":null}}`)
	if got, err := c.GenerationCost(context.Background(), "gen-1&x"); err != nil || got != (domain.GenerationCost{Cost: 0.5, PromptTokens: 12, CompletionTokens: 3, TokensKnown: true}) {
		t.Fatalf("normalized counts %+v, %v", got, err)
	}
	// With the person's own key the provider bills the inference too.
	c, _ = generationClient(t, http.StatusOK, `{"data":{"total_cost":0.001,"is_byok":true,"upstream_inference_cost":0.02}}`)
	if got, err := c.GenerationCost(context.Background(), "gen-1&x"); err != nil || math.Abs(got.Cost-0.021) > 1e-12 || got.TokensKnown {
		t.Fatalf("byok %+v, %v", got, err)
	}
}

// Stats not there yet, or without a cost, are pending; other failures say
// what failed without the key, and odd stats are refused.
func TestGenerationCostFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		status  int
		body    string
		pending bool
		text    string
	}{
		"not yet":      {status: http.StatusNotFound, body: `{"error":{"message":"Generation not found"}}`, pending: true},
		"no cost yet":  {status: http.StatusOK, body: `{"data":{"id":"gen-1","total_cost":null}}`, pending: true},
		"no data":      {status: http.StatusOK, body: `{}`, pending: true},
		"unauthorized": {status: http.StatusUnauthorized, body: `{"error":"test-secret"}`, text: "HTTP 401"},
		"malformed":    {status: http.StatusOK, body: `{"data":`, text: "malformed"},
		"negative":     {status: http.StatusOK, body: `{"data":{"total_cost":-1}}`, text: "malformed"},
		"too large":    {status: http.StatusOK, body: `{"data":{"model":"` + strings.Repeat("x", generationLimit) + `"}}`, text: "64 KiB"},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := generationClient(t, tc.status, tc.body)
			_, err := c.GenerationCost(context.Background(), "gen-1&x")
			if errors.Is(err, application.ErrGenerationPending) != tc.pending || err == nil || !strings.Contains(err.Error(), tc.text) || strings.Contains(err.Error(), "test-secret") {
				t.Fatalf("error %v", err)
			}
		})
	}
	if _, err := (GenerationCosts{}).GenerationCost(context.Background(), "gen-1"); err == nil {
		t.Fatal("a lookup ran without a key")
	}
	if _, err := (GenerationCosts{APIKey: "k"}).GenerationCost(context.Background(), " "); err == nil {
		t.Fatal("a lookup ran without an id")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (GenerationCosts{APIKey: "k"}).GenerationCost(cancelled, "gen-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	failing := &http.Client{Transport: catalogRoundTrip(func(*http.Request) (*http.Response, error) { return nil, errors.New("test-secret refused") })}
	if _, err := (GenerationCosts{APIKey: "test-secret", HTTPClient: failing}).GenerationCost(context.Background(), "gen-1"); err == nil || strings.Contains(err.Error(), "test-secret") {
		t.Fatalf("transport: %v", err)
	}
}
