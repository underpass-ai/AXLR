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

var testModelOptions = map[string]ModelOptions{
	"z-ai/glm-5.3-flash":         {Provider: json.RawMessage(`{"sort": "throughput", "max_price": {"completion": 1}}`), Reasoning: json.RawMessage(`{"effort":"low"}`), MaxTokens: 8192},
	"anthropic/claude-haiku-5.5": {Provider: json.RawMessage(`{"order":["anthropic"],"allow_fallbacks":false}`)},
	"local/qwen":                 {Provider: json.RawMessage(`{"sort":"price"}`), Reasoning: json.RawMessage(`{"effort":"high"}`), MaxTokens: 512},
}

// sentBody runs one Complete or Stream through a client and returns the
// request body it sent.
func sentBody(t *testing.T, config ClientConfig, model domain.ModelID, stream bool) string {
	t.Helper()
	var sent []byte
	config.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sent, _ = io.ReadAll(req.Body)
		if stream {
			return testResponse(200, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"), nil
		}
		return testResponse(200, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
	})}
	client, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	req := domain.CompletionRequest{Model: model, Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}}}
	if stream {
		_, err = client.Stream(context.Background(), req, func(domain.Text) error { return nil })
	} else {
		_, err = client.Complete(context.Background(), req)
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(sent)
}

// The options configured for a model reach OpenRouter in both Complete and
// Stream, in the prompt-cached body too; a client without options, another
// model and a local server receive the body byte for byte as before.
func TestClientSendsModelOptionsOnlyToOpenRouter(t *testing.T) {
	local := "http://127.0.0.1:8080/v1/chat/completions"
	for _, tc := range []struct {
		name     string
		config   ClientConfig
		model    domain.ModelID
		complete string
		stream   string
	}{
		{
			name:     "no options",
			config:   ClientConfig{APIKey: "test-secret"},
			model:    "z-ai/glm-5.3-flash",
			complete: `{"model":"z-ai/glm-5.3-flash","messages":[{"role":"user","content":"Hi"}],"stream":false}`,
			stream:   `{"model":"z-ai/glm-5.3-flash","messages":[{"role":"user","content":"Hi"}],"stream":true}`,
		},
		{
			name:     "configured model",
			config:   ClientConfig{APIKey: "test-secret", Models: testModelOptions},
			model:    "z-ai/glm-5.3-flash",
			complete: `{"model":"z-ai/glm-5.3-flash","messages":[{"role":"user","content":"Hi"}],"stream":false,"provider":{"sort":"throughput","max_price":{"completion":1}},"reasoning":{"effort":"low"},"max_tokens":8192}`,
			stream:   `{"model":"z-ai/glm-5.3-flash","messages":[{"role":"user","content":"Hi"}],"stream":true,"provider":{"sort":"throughput","max_price":{"completion":1}},"reasoning":{"effort":"low"},"max_tokens":8192}`,
		},
		{
			name:     "unconfigured model",
			config:   ClientConfig{APIKey: "test-secret", Models: testModelOptions},
			model:    "z-ai/glm-5.3-flash:nitro",
			complete: `{"model":"z-ai/glm-5.3-flash:nitro","messages":[{"role":"user","content":"Hi"}],"stream":false}`,
			stream:   `{"model":"z-ai/glm-5.3-flash:nitro","messages":[{"role":"user","content":"Hi"}],"stream":true}`,
		},
		{
			// The router rewrites a local id to the served name; options
			// under that very name still stay off a local server.
			name:     "local endpoint",
			config:   ClientConfig{Endpoint: local, Models: testModelOptions},
			model:    "local/qwen",
			complete: `{"model":"local/qwen","messages":[{"role":"user","content":"Hi"}],"stream":false}`,
			stream:   `{"model":"local/qwen","messages":[{"role":"user","content":"Hi"}],"stream":true,"stream_options":{"include_usage":true}}`,
		},
		{
			name:     "prompt cache",
			config:   ClientConfig{APIKey: "test-secret", Models: testModelOptions},
			model:    "anthropic/claude-haiku-5.5",
			complete: `{"model":"anthropic/claude-haiku-5.5","stream":false,"provider":{"order":["anthropic"],"allow_fallbacks":false},"messages":[{"role":"user","content":"Hi"}],"cache_control":{"type":"ephemeral"}}`,
			stream:   `{"model":"anthropic/claude-haiku-5.5","stream":true,"provider":{"order":["anthropic"],"allow_fallbacks":false},"messages":[{"role":"user","content":"Hi"}],"cache_control":{"type":"ephemeral"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sentBody(t, tc.config, tc.model, false); got != tc.complete {
				t.Fatalf("Complete sent\n%s\nwant\n%s", got, tc.complete)
			}
			if got := sentBody(t, tc.config, tc.model, true); got != tc.stream {
				t.Fatalf("Stream sent\n%s\nwant\n%s", got, tc.stream)
			}
		})
	}
}

// The client keeps its own copy: a later change to the caller's map or
// bytes does not reach the next request.
func TestClientCopiesModelOptions(t *testing.T) {
	provider := json.RawMessage(`{"sort":"throughput"}`)
	models := map[string]ModelOptions{"z-ai/glm-5.3-flash": {Provider: provider}}
	config := ClientConfig{APIKey: "test-secret", Models: models}
	var sent []byte
	config.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sent, _ = io.ReadAll(req.Body)
		return testResponse(200, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
	})}
	client, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	copy(provider, `{"sort":"price"     }`)
	models["z-ai/glm-5.3-flash"] = ModelOptions{MaxTokens: 1}
	if _, err := client.Complete(context.Background(), domain.CompletionRequest{Model: "z-ai/glm-5.3-flash", Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sent), `"provider":{"sort":"throughput"}`) || strings.Contains(string(sent), "max_tokens") {
		t.Fatalf("sent %s", sent)
	}
}

func TestNewRejectsUnreadableModelOptions(t *testing.T) {
	for name, options := range map[string]ModelOptions{
		"provider list":     {Provider: json.RawMessage(`["anthropic"]`)},
		"provider null":     {Provider: json.RawMessage(`null`)},
		"reasoning invalid": {Reasoning: json.RawMessage(`{effort`)},
		"negative cap":      {MaxTokens: -1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := New(ClientConfig{APIKey: "test-secret", Models: map[string]ModelOptions{"z-ai/glm-5.3-flash": options}}); err == nil {
				t.Fatal("unreadable options accepted")
			}
		})
	}
}
