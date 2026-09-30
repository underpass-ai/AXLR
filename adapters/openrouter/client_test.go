package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

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
