package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/application"
	"github.com/underpass-ai/AXLR/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func testResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func simpleCompletionRequest() domain.CompletionRequest {
	return domain.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}},
	}
}

func TestClientRejectsBlankKey(t *testing.T) {
	for _, key := range []string{"", "   "} {
		if _, err := New(ClientConfig{APIKey: key}); err == nil {
			t.Fatalf("blank key %q accepted", key)
		}
	}
}

func TestClientCompletesThroughUseCase(t *testing.T) {
	calls := 0
	client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != "https://openrouter.ai/api/v1/chat/completions" || req.Method != http.MethodPost {
			t.Fatalf("wrong request target: %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer test-secret" || req.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("wrong request headers: %+v", req.Header)
		}
		body, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		assertJSONEqual(t, body, `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hi"}],"stream":false}`)
		return testResponse(200, `{"choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"provider_extra":true}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := (application.CompleteModelUseCase{Models: client}).Execute(context.Background(), simpleCompletionRequest())
	if err != nil || got.Message.Content != "Hello" || calls != 1 {
		t.Fatalf("completion = %+v, %v; calls = %d", got, err, calls)
	}
}

func TestClientRejectsMalformedAndOversizedReplies(t *testing.T) {
	for name, body := range map[string]string{
		"malformed JSON": "{bad",
		"no choices":     `{"choices":[]}`,
		"oversized":      strings.Repeat("x", 8*1024*1024+1),
	} {
		t.Run(name, func(t *testing.T) {
			client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return testResponse(200, body), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.Complete(context.Background(), simpleCompletionRequest()); err == nil {
				t.Fatal("malformed response accepted")
			}
		})
	}
}

func TestClientClassifiesProviderErrorsWithoutLeakingBody(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   ProviderErrorCategory
	}{
		{400, CategoryInvalidRequest}, {422, CategoryInvalidRequest},
		{401, CategoryAuthentication}, {403, CategoryAuthentication},
		{402, CategoryInsufficientCredits}, {429, CategoryRateLimited},
		{500, CategoryProviderFailure}, {503, CategoryProviderFailure},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			calls := 0
			client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return testResponse(tc.status, `{"error":"raw-sensitive-body"}`), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Complete(context.Background(), simpleCompletionRequest())
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.StatusCode != tc.status || providerErr.Category != tc.want || calls != 1 {
				t.Fatalf("provider error = %v; calls = %d", err, calls)
			}
			if strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), "raw-sensitive-body") {
				t.Fatalf("sensitive value leaked: %v", err)
			}
		})
	}
}

// An HTTP error keeps what the provider said, on one line, bounded, without
// the client's key or anything that looks like a credential, and says what
// the person can do about it. On 12 October 2026 a wrong model id read only
// "OpenRouter invalid_request (HTTP 400)".
func TestClientKeepsTheProvidersErrorMessage(t *testing.T) {
	for _, tc := range []struct {
		status    int
		body      string
		endpoint  string
		want      string
		hint      ProviderHint
		hintWords string
	}{
		{400, `{"error":{"code":400,"message":"anthropic/claude-nonexistent-9 is not a valid model ID"}}`, "", "OpenRouter invalid_request (HTTP 400): anthropic/claude-nonexistent-9 is not a valid model ID", HintChooseModel, "/model"},
		{400, `{"error":{"code":400,"message":"This endpoint's maximum context length is 200000 tokens."}}`, "", "OpenRouter invalid_request (HTTP 400): This endpoint's maximum context length is 200000 tokens.", HintNone, ""},
		{401, `{"error":{"code":401,"message":"No auth credentials found for test-secret\nBearer abc.def"}}`, "", "OpenRouter authentication (HTTP 401): No auth credentials found for [redacted] [redacted]", HintCheckKey, "OPENROUTER_API_KEY"},
		{402, `{"error":{"code":402,"message":"Insufficient credits"}}`, "", "OpenRouter insufficient_credits (HTTP 402): Insufficient credits", HintCredits, "credits"},
		{429, `{"error":{"code":429,"message":"Rate limit exceeded"}}`, "", "OpenRouter rate_limited (HTTP 429): Rate limit exceeded", HintRetryLater, "wait"},
		{404, `{"object":"error","message":"The model ` + "`qwen-x`" + ` does not exist.","type":"NotFoundError","code":404}`, "http://127.0.0.1:8080/v1/chat/completions", "model endpoint 127.0.0.1:8080 invalid_request (HTTP 404): The model `qwen-x` does not exist.", HintChooseModel, "/model"},
		{503, `<html>Service Unavailable</html>`, "", "OpenRouter provider_failure (HTTP 503)", HintNone, ""},
	} {
		t.Run(tc.want, func(t *testing.T) {
			client, err := New(ClientConfig{APIKey: "test-secret", Endpoint: tc.endpoint, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return testResponse(tc.status, tc.body), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			for _, call := range []func() error{
				func() error { _, err := client.Complete(context.Background(), simpleCompletionRequest()); return err },
				func() error {
					_, err := client.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
					return err
				},
			} {
				err := call()
				var providerErr *ProviderError
				if !errors.As(err, &providerErr) || err.Error() != tc.want || providerErr.HintKind() != tc.hint || !strings.Contains(providerErr.Hint(), tc.hintWords) {
					t.Fatalf("error = %v, hint %q %q", err, providerErr.HintKind(), providerErr.Hint())
				}
			}
		})
	}
	long := sanitizeProviderMessage(strings.Repeat("word ", 200) + " sk-or-v1-0123456789abcdef")
	if utf8.RuneCountInString(long) != MaxProviderMessageRunes || !strings.HasSuffix(long, "…") {
		t.Fatalf("bounded message %d runes: %q", utf8.RuneCountInString(long), long)
	}
	if got := sanitizeProviderMessage("key sk-or-v1-0123456789abcdef and " + strings.Repeat("A", 48)); got != "key [redacted] and [redacted]" {
		t.Fatalf("redacted %q", got)
	}
}

func TestClientAvoidsNetworkAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return testResponse(200, `{}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(ctx, simpleCompletionRequest()); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("canceled completion = %v; calls = %d", err, calls)
	}
}

func TestClientWrapsTransportError(t *testing.T) {
	client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), simpleCompletionRequest())
	var transportErr *TransportError
	if !errors.As(err, &transportErr) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("transport error = %v", err)
	}
}

func TestClientRefusesRedirect(t *testing.T) {
	calls := 0
	client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		response := testResponse(302, "")
		response.Header.Set("Location", "https://example.com/collect")
		return response, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), simpleCompletionRequest()); err == nil || calls != 1 {
		t.Fatalf("redirect result = %v; calls = %d", err, calls)
	}
}

func TestClientAllowsHistoricalToolResultWithoutCurrentDefinition(t *testing.T) {
	calls := 0
	client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return testResponse(200, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	req := domain.CompletionRequest{
		Model: "openai/gpt-4o",
		Messages: []domain.Message{
			{Role: domain.RoleUser, Content: "Find it"},
			{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{ID: "call_1", Name: "search", Arguments: testObject(t, `{}`)}}},
			{Role: domain.RoleTool, ToolCallID: "call_1", Content: "Found"},
		},
	}
	if _, err := client.Complete(context.Background(), req); err == nil || calls != 0 {
		t.Fatalf("follow-up omitted current tools: %v; HTTP calls = %d", err, calls)
	}
	req.Tools = []domain.ToolDefinition{{Name: "different_current_tool", Description: "current capability", Parameters: testObject(t, `{"type":"object"}`)}}
	if _, err := client.Complete(context.Background(), req); err != nil || calls != 1 {
		t.Fatalf("historical or rejected call blocked: %v; HTTP calls = %d", err, calls)
	}
}

// OpenRouter answers an error found after it accepted the request with HTTP
// 200 and the error in the body: the code classifies it as a status would,
// the message is kept, and a local server's error names the server.
func TestClientClassifiesErrorBodyWithStatus200(t *testing.T) {
	for _, tc := range []struct {
		endpoint, body, want string
		status               int
		category             ProviderErrorCategory
	}{
		{"", `{"error":{"code":429,"message":"raw-sensitive-body"}}`, "OpenRouter rate_limited (HTTP 429): raw-sensitive-body", 429, CategoryRateLimited},
		{"", `{"error":{"message":"raw-sensitive-body"}}`, "OpenRouter provider_failure (HTTP 502): raw-sensitive-body", 502, CategoryProviderFailure},
		{"http://127.0.0.1:8080/v1/chat/completions", `{"error":{"code":400,"message":"raw-sensitive-body"},"choices":[]}`, "model endpoint 127.0.0.1:8080 invalid_request (HTTP 400): raw-sensitive-body", 400, CategoryInvalidRequest},
	} {
		t.Run(tc.want, func(t *testing.T) {
			client, err := New(ClientConfig{APIKey: "test-secret", Endpoint: tc.endpoint, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return testResponse(200, tc.body), nil
			})}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Complete(context.Background(), simpleCompletionRequest())
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.StatusCode != tc.status || providerErr.Category != tc.category || err.Error() != tc.want {
				t.Fatalf("error = %T %v", err, err)
			}
		})
	}
	client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testResponse(200, `{"error":null,"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := client.Complete(context.Background(), simpleCompletionRequest()); err != nil || got.Message.Content != "ok" {
		t.Fatalf("null error refused: %+v %v", got, err)
	}
}
