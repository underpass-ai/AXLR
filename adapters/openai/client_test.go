package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/adapters/provider"
	"github.com/underpass-ai/AXLR/application"
	"github.com/underpass-ai/AXLR/domain"
)

var _ application.ModelStreamPort = (*Client)(nil)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type trackedStreamBody struct {
	io.Reader
	closed bool
}

func (b *trackedStreamBody) Close() error { b.closed = true; return nil }

// sse frames the events of a Responses stream as the API sends them, each
// with its event name, data and sequence number.
func sse(events ...string) string {
	var out strings.Builder
	for i, data := range events {
		var event struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(data), &event)
		numbered := strings.TrimSuffix(data, "}") + `,"sequence_number":` + itoa(i) + "}"
		out.WriteString("event: " + event.Type + "\ndata: " + numbered + "\n\n")
	}
	return out.String()
}

func itoa(i int) string { return string(rune('0' + i%10)) }

func simpleRequest() domain.CompletionRequest {
	return domain.CompletionRequest{Model: "gpt-5", Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}}}
}

func testClient(t *testing.T, config ClientConfig, status int, body io.ReadCloser, inspect func(*http.Request, []byte)) (*Client, *int) {
	t.Helper()
	calls := new(int)
	config.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		*calls++
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if inspect != nil {
			inspect(r, raw)
		}
		return &http.Response{StatusCode: status, Body: body, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}, nil
	})}
	if config.APIKey == "" {
		config.APIKey = "sk-proj-secret"
	}
	c, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return c, calls
}

func decode(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("request body %s: %v", raw, err)
	}
	return body
}

const reasoningItem = `{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"Comparing the files"}],"encrypted_content":"gAAAAABo"}`

// The response gpt-5 would send for a turn that reasons, says a word and
// calls two functions.
func finalResponse(status string) string {
	return `{"id":"resp_1","object":"response","status":"` + status + `","error":null,"incomplete_details":null,"model":"gpt-5",` +
		`"output":[` + reasoningItem + `,{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"hé🌍","annotations":[]}]},` +
		`{"type":"function_call","id":"fc_1","call_id":"call_a","name":"exec","arguments":"{\"command\":\"pwd\"}","status":"completed"},` +
		`{"type":"function_call","id":"fc_2","call_id":"call_b","name":"read","arguments":"{\"path\":\"README.md\"}","status":"completed"}],` +
		`"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":40},"output_tokens":50,"output_tokens_details":{"reasoning_tokens":30},"total_tokens":150}}`
}

var toolStream = []string{
	`{"type":"response.created","response":{"id":"resp_1","object":"response","status":"in_progress","output":[]}}`,
	`{"type":"response.in_progress","response":{"id":"resp_1","object":"response","status":"in_progress","output":[]}}`,
	`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1","summary":[]}}`,
	`{"type":"response.reasoning_summary_part.added","item_id":"rs_1","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`,
	`{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","output_index":0,"summary_index":0,"delta":"Comparing the files"}`,
	`{"type":"response.reasoning_summary_text.done","item_id":"rs_1","output_index":0,"summary_index":0,"text":"Comparing the files"}`,
	`{"type":"response.output_item.done","output_index":0,"item":` + reasoningItem + `}`,
	`{"type":"response.output_item.added","output_index":1,"item":{"type":"message","id":"msg_1","status":"in_progress","role":"assistant","content":[]}}`,
	`{"type":"response.content_part.added","item_id":"msg_1","output_index":1,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
	`{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"content_index":0,"delta":"hé"}`,
	`{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"content_index":0,"delta":"🌍"}`,
	`{"type":"response.output_text.done","item_id":"msg_1","output_index":1,"content_index":0,"text":"hé🌍"}`,
	`{"type":"response.output_item.done","output_index":1,"item":{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"hé🌍","annotations":[]}]}}`,
	`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","id":"fc_1","call_id":"call_a","name":"exec","arguments":"","status":"in_progress"}}`,
	`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":2,"delta":"{\"command\":"}`,
	`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":2,"delta":"\"pwd\"}"}`,
	`{"type":"response.function_call_arguments.done","item_id":"fc_1","output_index":2,"arguments":"{\"command\":\"pwd\"}"}`,
	`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","id":"fc_1","call_id":"call_a","name":"exec","arguments":"{\"command\":\"pwd\"}","status":"completed"}}`,
	`{"type":"response.output_item.added","output_index":3,"item":{"type":"function_call","id":"fc_2","call_id":"call_b","name":"read","arguments":"","status":"in_progress"}}`,
	`{"type":"response.function_call_arguments.done","item_id":"fc_2","output_index":3,"arguments":"{\"path\":\"README.md\"}"}`,
	`{"type":"response.output_item.done","output_index":3,"item":{"type":"function_call","id":"fc_2","call_id":"call_b","name":"read","arguments":"{\"path\":\"README.md\"}","status":"completed"}}`,
	`{"type":"response.completed","response":` + finalResponse("completed") + `}`,
}

// The request carries the Responses shapes: a bearer token, flat function
// tools, no server state and encrypted reasoning asked for, every message an
// item in order, the assistant's reasoning items replayed byte for byte
// before its text and its function calls, and a tool result as the call's
// output.
func TestRequestMapsItemsToolsAndReasoning(t *testing.T) {
	reasoning, _ := domain.NewJSONValue([]byte(`[` + reasoningItem + `]`))
	arguments, _ := domain.NewJSONObject([]byte(`{"path":"a"}`))
	parameters, _ := domain.NewJSONObject([]byte(`{"type":"object","properties":{"path":{"type":"string"}}}`))
	req := domain.CompletionRequest{Model: "gpt-5", Messages: []domain.Message{
		{Role: domain.RoleSystem, Content: "Be brief."},
		{Role: domain.RoleUser, Content: "Read a"},
		{Role: domain.RoleAssistant, Content: "Reading.", Reasoning: reasoning, ToolCalls: []domain.ToolCall{{ID: "call_1", Name: "read", Arguments: arguments}}},
		{Role: domain.RoleTool, ToolCallID: "call_1", Content: "one"},
	}, Tools: []domain.ToolDefinition{{Name: "read", Description: "Read a file", Parameters: parameters}}}
	var sent []byte
	var headers http.Header
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(toolStream...))}, func(r *http.Request, raw []byte) {
		sent, headers = raw, r.Header.Clone()
	})
	if _, err := c.Stream(context.Background(), req, func(domain.Text) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if headers.Get("Authorization") != "Bearer sk-proj-secret" || headers.Get("Accept") != "text/event-stream" || headers.Get("Content-Type") != "application/json" {
		t.Fatalf("headers %v", headers)
	}
	body := decode(t, sent)
	want := decode(t, []byte(`{
		"model":"gpt-5","stream":true,"store":false,
		"include":["reasoning.encrypted_content"],
		"reasoning":{"summary":"auto"},
		"input":[
		  {"role":"system","content":"Be brief."},
		  {"role":"user","content":"Read a"},
		  `+reasoningItem+`,
		  {"role":"assistant","content":"Reading."},
		  {"type":"function_call","call_id":"call_1","name":"read","arguments":"{\"path\":\"a\"}"},
		  {"type":"function_call_output","call_id":"call_1","output":"one"}
		],
		"tools":[{"type":"function","name":"read","description":"Read a file","parameters":{"type":"object","properties":{"path":{"type":"string"}}}}]
	}`))
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("request\n%s\nwant\n%v", sent, want)
	}
}

// thinking: false sends neither the reasoning parameter nor the include, for
// a model without reasoning.
func TestRequestWithoutReasoning(t *testing.T) {
	off := false
	var sent []byte
	c, _ := testClient(t, ClientConfig{Thinking: &off}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(toolStream...))}, func(_ *http.Request, raw []byte) { sent = raw })
	if _, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil }); err != nil {
		t.Fatal(err)
	}
	body := decode(t, sent)
	if _, present := body["reasoning"]; present {
		t.Fatalf("reasoning sent: %s", sent)
	}
	if _, present := body["include"]; present || body["store"] != false {
		t.Fatalf("request %s", sent)
	}
}

func TestStreamTextReasoningAndParallelFunctionCalls(t *testing.T) {
	body := &trackedStreamBody{Reader: strings.NewReader(sse(toolStream...))}
	c, calls := testClient(t, ClientConfig{}, 200, body, nil)
	var deltas []domain.Text
	got, err := c.Stream(context.Background(), simpleRequest(), func(text domain.Text) error { deltas = append(deltas, text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Content != "hé🌍" || len(deltas) != 2 || deltas[0] != "hé" || deltas[1] != "🌍" || got.FinishReason != "tool_calls" || got.Provider != "api.openai.com" || got.RequestBytes == 0 {
		t.Fatalf("result = %+v; deltas = %v", got, deltas)
	}
	if len(got.Message.ToolCalls) != 2 {
		t.Fatalf("calls = %v", got.Message.ToolCalls)
	}
	a, b := got.Message.ToolCalls[0], got.Message.ToolCalls[1]
	if a.ID != "call_a" || a.Name != "exec" || string(a.Arguments.Bytes()) != `{"command":"pwd"}` || b.ID != "call_b" || b.Name != "read" || string(b.Arguments.Bytes()) != `{"path":"README.md"}` {
		t.Fatalf("calls = %+v", got.Message.ToolCalls)
	}
	if string(got.Message.Reasoning.Bytes()) != `[`+reasoningItem+`]` {
		t.Fatalf("reasoning = %s", got.Message.Reasoning.Bytes())
	}
	usage := got.Usage
	if usage == nil || usage.PromptTokens != 100 || usage.CachedTokens != 40 || usage.CacheWriteTokens != 0 || usage.CompletionTokens != 50 || usage.ReasoningTokens != 30 || usage.TotalTokens != 150 || usage.CostKnown {
		t.Fatalf("usage = %+v", usage)
	}
	if *calls != 1 || !body.closed {
		t.Fatalf("requests=%d closed=%v", *calls, body.closed)
	}
}

// The reasoning item a stream returned goes back in the next request, before
// the assistant's text and calls, unmodified.
func TestStreamedReasoningGoesBackInTheNextRequest(t *testing.T) {
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(toolStream...))}, nil)
	first, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	next := domain.CompletionRequest{Model: "gpt-5", Messages: []domain.Message{
		{Role: domain.RoleUser, Content: "Hi"},
		first.Message,
		{Role: domain.RoleTool, ToolCallID: "call_a", Content: "/home"},
		{Role: domain.RoleTool, ToolCallID: "call_b", Content: "# AXLR"},
	}}
	var sent []byte
	c, _ = testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(toolStream...))}, func(_ *http.Request, raw []byte) { sent = raw })
	if _, err := c.Stream(context.Background(), next, func(domain.Text) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Input []json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(sent, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Input) != 7 || string(body.Input[1]) != reasoningItem || !strings.Contains(string(body.Input[2]), `"role":"assistant"`) || !strings.Contains(string(body.Input[3]), `"call_id":"call_a"`) || !strings.Contains(string(body.Input[5]), `"function_call_output"`) {
		t.Fatalf("input %s", sent)
	}
}

// A terminal event without output is read from the items the stream
// finished one by one.
func TestStreamCompletedWithoutOutputUsesTheDoneItems(t *testing.T) {
	events := append([]string(nil), toolStream[:len(toolStream)-1]...)
	events = append(events, `{"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`)
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(events...))}, nil)
	got, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Content != "hé🌍" || len(got.Message.ToolCalls) != 2 || string(got.Message.Reasoning.Bytes()) != `[`+reasoningItem+`]` || got.Usage.TotalTokens != 3 {
		t.Fatalf("result = %+v", got)
	}
}

// An answer the output limit cut short ends with the note, streamed too; a
// reasoning item without encrypted content is not kept, since a stateless
// request could not send it back.
func TestStreamIncompleteAddsTheOutputLimitNote(t *testing.T) {
	events := []string{
		toolStream[0],
		`{"type":"response.output_text.delta","item_id":"msg_1","output_index":1,"content_index":0,"delta":"Half an"}`,
		`{"type":"response.incomplete","response":{"id":"resp_1","object":"response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"reasoning","id":"rs_1","summary":[]},{"type":"message","id":"msg_1","status":"incomplete","role":"assistant","content":[{"type":"output_text","text":"Half an","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":4096,"total_tokens":4097}}}`,
	}
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(events...))}, nil)
	var streamed string
	got, err := c.Stream(context.Background(), simpleRequest(), func(text domain.Text) error { streamed += string(text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.FinishReason != "length" || got.Message.Content != "Half an\n\n"+provider.OutputLimitNote || streamed != string(got.Message.Content) || got.Message.HasReasoning() {
		t.Fatalf("result = %+v streamed %q", got, streamed)
	}
}

// A refusal part says so in the message.
func TestStreamRefusalKeepsTheText(t *testing.T) {
	events := []string{
		toolStream[0],
		`{"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"refusal","refusal":"I can't help with that."}]}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`,
	}
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(events...))}, nil)
	got, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.FinishReason != "stop" || !strings.Contains(string(got.Message.Content), "declined") || !strings.Contains(string(got.Message.Content), "I can't help") {
		t.Fatalf("result = %+v", got)
	}
}

func TestStreamRejectsIncompleteMalformedAndOversized(t *testing.T) {
	for name, wire := range map[string]string{
		"missing completed": sse(toolStream[:len(toolStream)-1]...),
		"malformed":         sse(toolStream[0]) + "event: x\ndata: {bad\n\n",
		"empty response":    sse(toolStream[0], `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[]}}`),
		"no response":       sse(toolStream[0], `{"type":"response.completed"}`),
		"oversized event":   sse(`{"type":"response.created","pad":"` + strings.Repeat("x", 1024*1024) + `"}`),
		"invalid UTF-8":     sse(toolStream[0]) + "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + string([]byte{0xff}) + "\"}\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			body := &trackedStreamBody{Reader: strings.NewReader(wire)}
			c, calls := testClient(t, ClientConfig{}, 200, body, nil)
			if _, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil }); err == nil {
				t.Fatal("invalid stream accepted")
			}
			if *calls != 1 || !body.closed {
				t.Fatalf("requests=%d closed=%v", *calls, body.closed)
			}
		})
	}
}

// A failed response and a mid-stream error event are the provider's error,
// classified by its code, named after the endpoint and without the key.
func TestStreamFailedAndErrorEvents(t *testing.T) {
	for name, tc := range map[string]struct {
		event    string
		status   int
		category provider.ProviderErrorCategory
	}{
		"failed":       {`{"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"server_error","message":"boom sk-proj-secret"},"output":[]}}`, 500, provider.CategoryProviderFailure},
		"error event":  {`{"type":"error","code":"rate_limit_exceeded","message":"slow down","param":null}`, 429, provider.CategoryRateLimited},
		"quota event":  {`{"type":"error","code":"insufficient_quota","message":"You exceeded your current quota","param":null}`, 402, provider.CategoryInsufficientCredits},
		"unknown code": {`{"type":"error","code":null,"message":"odd","param":null}`, 502, provider.CategoryProviderFailure},
	} {
		c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(toolStream[0], tc.event))}, nil)
		got, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
		var failed *provider.ProviderError
		if !errors.As(err, &failed) || failed.StatusCode != tc.status || failed.Category != tc.category || failed.Provider != "model endpoint api.openai.com" || strings.Contains(err.Error(), "sk-proj-secret") {
			t.Fatalf("%s: err = %v (%+v)", name, err, failed)
		}
		if got.RequestBytes == 0 {
			t.Fatalf("%s: sent = %+v", name, got)
		}
	}
}

// HTTP errors keep the provider's message, classified by status and code,
// redacted, with the hint the console shows.
func TestHTTPErrorsAreClassifiedWithoutTheKey(t *testing.T) {
	for _, tc := range []struct {
		status   int
		body     string
		want     int
		category provider.ProviderErrorCategory
		hint     provider.ProviderHint
		message  string
	}{
		{401, `{"error":{"message":"Incorrect API key provided: sk-proj-secret","type":"invalid_request_error","param":null,"code":"invalid_api_key"}}`, 401, provider.CategoryAuthentication, provider.HintCheckKey, "Incorrect API key provided: [redacted]"},
		{429, `{"error":{"message":"You exceeded your current quota","type":"insufficient_quota","param":null,"code":"insufficient_quota"}}`, 402, provider.CategoryInsufficientCredits, provider.HintCredits, "You exceeded your current quota"},
		{429, `{"error":{"message":"Rate limit reached","type":"requests","param":null,"code":"rate_limit_exceeded"}}`, 429, provider.CategoryRateLimited, provider.HintRetryLater, "Rate limit reached"},
		{404, `{"error":{"message":"The model ` + "`gpt-9`" + ` does not exist or you do not have access to it.","type":"invalid_request_error","param":null,"code":"model_not_found"}}`, 404, provider.CategoryInvalidRequest, provider.HintChooseModel, "The model `gpt-9` does not exist or you do not have access to it."},
		{400, `{"error":{"message":"Unsupported parameter: 'reasoning'","type":"invalid_request_error","param":"reasoning","code":"unsupported_parameter"}}`, 400, provider.CategoryInvalidRequest, provider.HintNone, "Unsupported parameter: 'reasoning'"},
		{503, `<html>oops</html>`, 503, provider.CategoryProviderFailure, provider.HintNone, ""},
	} {
		for name, call := range map[string]func(*Client) (domain.CompletionResult, error){
			"stream": func(c *Client) (domain.CompletionResult, error) {
				return c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
			},
			"complete": func(c *Client) (domain.CompletionResult, error) {
				return c.Complete(context.Background(), simpleRequest())
			},
		} {
			c, _ := testClient(t, ClientConfig{}, tc.status, io.NopCloser(strings.NewReader(tc.body)), nil)
			got, err := call(c)
			var failed *provider.ProviderError
			if !errors.As(err, &failed) || failed.StatusCode != tc.want || failed.Category != tc.category || failed.HintKind() != tc.hint || failed.Message != tc.message || failed.Provider != "model endpoint api.openai.com" {
				t.Fatalf("%s HTTP %d: %v (%+v)", name, tc.status, err, failed)
			}
			if strings.Contains(err.Error(), "sk-proj-secret") || got.RequestBytes == 0 {
				t.Fatalf("%s HTTP %d leaked or lost the request: %v %+v", name, tc.status, err, got)
			}
		}
	}
}

// A whole response maps like a stream, its reasoning kept byte for byte.
func TestCompleteMapsAWholeResponse(t *testing.T) {
	var sent []byte
	var headers http.Header
	c, _ := testClient(t, ClientConfig{}, 200, io.NopCloser(strings.NewReader(finalResponse("completed"))), func(r *http.Request, raw []byte) { sent, headers = raw, r.Header.Clone() })
	got, err := c.Complete(context.Background(), simpleRequest())
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("Accept") == "text/event-stream" || strings.Contains(string(sent), `"stream"`) {
		t.Fatalf("whole reply asked for a stream: %s %v", sent, headers)
	}
	if got.Message.Content != "hé🌍" || len(got.Message.ToolCalls) != 2 || string(got.Message.Reasoning.Bytes()) != `[`+reasoningItem+`]` || got.FinishReason != "tool_calls" || got.Usage.PromptTokens != 100 || got.Usage.ReasoningTokens != 30 || got.RequestBytes != len(sent) {
		t.Fatalf("result = %+v reasoning %s", got, got.Message.Reasoning.Bytes())
	}
	c, _ = testClient(t, ClientConfig{}, 200, io.NopCloser(strings.NewReader(`{"id":"resp_2","status":"failed","error":{"code":"server_error","message":"boom"},"output":[]}`)), nil)
	var failed *provider.ProviderError
	if _, err := c.Complete(context.Background(), simpleRequest()); !errors.As(err, &failed) || failed.StatusCode != 500 || failed.Provider != "model endpoint api.openai.com" {
		t.Fatalf("failed response with HTTP 200: %v", err)
	}
}

func TestNewChecksKeyAndEndpoint(t *testing.T) {
	if _, err := New(ClientConfig{}); err == nil || !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("no key: %v", err)
	}
	if _, err := New(ClientConfig{Endpoint: "http://127.0.0.1:8000/v1/responses"}); err != nil {
		t.Fatalf("loopback without key: %v", err)
	}
	if _, err := New(ClientConfig{APIKey: "k", Endpoint: "http://proxy.example.com/v1/responses"}); err == nil {
		t.Fatal("plain http off loopback accepted")
	}
	c, err := New(ClientConfig{APIKey: "k", Endpoint: "https://proxy.example.com/openai/v1/responses"})
	if err != nil || c.provider != "model endpoint proxy.example.com" || c.host != "proxy.example.com" {
		t.Fatalf("client %+v %v", c, err)
	}
}

func TestStreamStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(toolStream...))}, nil)
	_, err := c.Stream(ctx, simpleRequest(), func(domain.Text) error { cancel(); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
