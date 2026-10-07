package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
)

func TestLoopbackEndpointNeedsNoKeyAndSendsNoAuthorization(t *testing.T) {
	const local = "http://127.0.0.1:8080/v1/chat/completions"
	wire := streamEvents(
		`{"choices":[{"finish_reason":null,"index":0,"delta":{"role":"assistant","content":null}}],"model":"/models/qwen.gguf"}`,
		`{"choices":[{"finish_reason":null,"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"local_read","arguments":"{"}}]}}]}`,
		`{"choices":[{"finish_reason":null,"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"path\":\"README.md\"}"}}]}}]}`,
		`{"choices":[{"finish_reason":"tool_calls","index":0,"delta":{}}]}`,
		`{"choices":[],"usage":{"completion_tokens":45,"prompt_tokens":318,"total_tokens":363},"timings":{"prompt_n":4}}`,
		"[DONE]")
	client, err := New(ClientConfig{Endpoint: local, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != local {
			t.Fatalf("target = %s", r.URL)
		}
		if _, ok := r.Header["Authorization"]; ok {
			t.Fatal("loopback request without a key carried an Authorization header")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, raw, `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hi"}],"stream":true,"stream_options":{"include_usage":true}}`)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(wire)), Header: make(http.Header)}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Message.ToolCalls) != 1 || string(got.Message.ToolCalls[0].Arguments.Bytes()) != `{"path":"README.md"}` || got.Usage == nil || got.Usage.TotalTokens != 363 {
		t.Fatalf("result = %+v", got)
	}
}

func TestEndpointKeyIsSentWhenConfigured(t *testing.T) {
	client, err := New(ClientConfig{APIKey: " local-secret ", Endpoint: "https://models.example.test/v1/chat/completions", HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer local-secret" || r.URL.Host != "models.example.test" {
			t.Fatalf("request = %s %v", r.URL, r.Header)
		}
		return testResponse(200, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), simpleCompletionRequest()); err != nil {
		t.Fatal(err)
	}
}

func TestEndpointValidation(t *testing.T) {
	for name, config := range map[string]ClientConfig{
		"remote without key":   {Endpoint: "https://models.example.test/v1/chat/completions"},
		"remote over http":     {APIKey: "k", Endpoint: "http://10.0.0.5:8000/v1/chat/completions"},
		"relative":             {Endpoint: "/v1/chat/completions"},
		"credentials in url":   {Endpoint: "http://user:pass@127.0.0.1:8080/v1/chat/completions"},
		"query":                {Endpoint: "http://127.0.0.1:8080/v1/chat/completions?x=1"},
		"other scheme":         {Endpoint: "ftp://127.0.0.1/v1/chat/completions"},
		"key with line breaks": {APIKey: "a\nb", Endpoint: "http://127.0.0.1:8080/v1/chat/completions"},
		"default without key":  {},
	} {
		if _, err := New(config); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for _, raw := range []string{"http://localhost:8080/v1/chat/completions", "http://127.0.0.2/v1/chat/completions", "http://[::1]:8000/v1/chat/completions"} {
		if _, err := New(ClientConfig{Endpoint: raw}); err != nil {
			t.Errorf("%s refused: %v", raw, err)
		}
	}
	if IsLoopbackEndpoint("http://localhost.example.test/") || IsLoopbackEndpoint("http://192.168.1.2/") || !IsLoopbackEndpoint("https://LOCALHOST/v1") {
		t.Fatal("loopback detection is wrong")
	}
}

func TestEndpointErrorsNameTheEndpoint(t *testing.T) {
	client, err := New(ClientConfig{Endpoint: "http://127.0.0.1:8080/v1/chat/completions", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testResponse(400, `{"error":"tools not supported"}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), simpleCompletionRequest())
	var provider *ProviderError
	if !errors.As(err, &provider) || provider.Category != CategoryInvalidRequest || err.Error() != "model endpoint 127.0.0.1:8080 invalid_request (HTTP 400)" {
		t.Fatalf("error = %v", err)
	}
	failing, err := New(ClientConfig{Endpoint: "http://127.0.0.1:8080/v1/chat/completions", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = failing.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
	if err == nil || err.Error() != "model endpoint 127.0.0.1:8080 transport failure" {
		t.Fatalf("error = %v", err)
	}
}
