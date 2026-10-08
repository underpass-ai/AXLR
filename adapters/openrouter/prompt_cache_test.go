package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
)

func cacheRequest(model domain.ModelID) domain.CompletionRequest {
	return domain.CompletionRequest{
		Model:    model,
		Messages: []domain.Message{{Role: domain.RoleSystem, Content: "You are AXLR."}, {Role: domain.RoleUser, Content: "Hi"}},
	}
}

// An Anthropic model through OpenRouter is marked for the prompt cache: a
// top-level cache_control and a breakpoint on the system prompt, which
// becomes content blocks; every other message keeps its string content.
func TestPromptCacheBodyMarksAnthropicModelsOnOpenRouter(t *testing.T) {
	wire, err := mapRequest(cacheRequest("anthropic/claude-haiku-5.5"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(promptCacheBody(wire, true))
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, encoded, `{
		"model": "anthropic/claude-haiku-5.5",
		"cache_control": {"type": "ephemeral"},
		"messages": [
			{"role": "system", "content": [{"type": "text", "text": "You are AXLR.", "cache_control": {"type": "ephemeral"}}]},
			{"role": "user", "content": "Hi"}
		],
		"stream": false
	}`)
}

// Other models on OpenRouter, and Anthropic ids on a local server, receive
// the request unchanged.
func TestPromptCacheBodyLeavesOtherModelsAndEndpointsAlone(t *testing.T) {
	for _, tc := range []struct {
		model      domain.ModelID
		openRouter bool
	}{{"openai/gpt-4o", true}, {"z-ai/glm-5.3-flash", true}, {"anthropic/claude-haiku-5.5", false}, {"local/qwen", false}} {
		wire, err := mapRequest(cacheRequest(tc.model))
		if err != nil {
			t.Fatal(err)
		}
		plain, _ := json.Marshal(wire)
		encoded, err := json.Marshal(promptCacheBody(wire, tc.openRouter))
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != string(plain) || strings.Contains(string(encoded), "cache_control") {
			t.Fatalf("%s on OpenRouter=%v: body changed: %s", tc.model, tc.openRouter, encoded)
		}
	}
}

// A request without a system prompt still asks for the automatic breakpoint.
func TestPromptCacheBodyWithoutSystemPrompt(t *testing.T) {
	wire, err := mapRequest(domain.CompletionRequest{Model: "anthropic/claude-sonnet-5.5", Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(promptCacheBody(wire, true))
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, encoded, `{"model": "anthropic/claude-sonnet-5.5", "cache_control": {"type": "ephemeral"}, "messages": [{"role": "user", "content": "Hi"}], "stream": false}`)
}

// The client marks the body it sends to OpenRouter and reads the cache
// counts the usage reports.
func TestClientSendsCacheMarksAndReadsCacheUsage(t *testing.T) {
	var sent map[string]any
	client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Fatal(err)
		}
		return testResponse(200, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1000,"completion_tokens":5,"total_tokens":1005,"prompt_tokens_details":{"cached_tokens":900,"cache_write_tokens":50}}}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Complete(context.Background(), cacheRequest("anthropic/claude-haiku-5.5"))
	if err != nil {
		t.Fatal(err)
	}
	if sent["cache_control"] == nil {
		t.Fatalf("request not marked: %v", sent)
	}
	if result.Usage == nil || result.Usage.CachedTokens != 900 || result.Usage.CacheWriteTokens != 50 {
		t.Fatalf("usage = %+v", result.Usage)
	}
}
