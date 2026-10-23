package anthropic

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

// sse frames the events of a Messages stream as the API sends them, each
// with its event name and data.
func sse(events ...string) string {
	var out strings.Builder
	for _, data := range events {
		var event struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(data), &event)
		out.WriteString("event: " + event.Type + "\ndata: " + data + "\n\n")
	}
	return out.String()
}

func simpleRequest() domain.CompletionRequest {
	return domain.CompletionRequest{Model: "claude-sonnet-5-5", Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}}}
}

// testClient answers every request with status and body, handing the
// request to inspect for its headers and body.
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
		config.APIKey = "sk-ant-secret"
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

const thinkingBlock = `{"type":"thinking","thinking":"Comparing the files","signature":"CAQSqwc"}`

// The request carries Anthropic's headers and shapes: the key as x-api-key,
// the system prompt at the top with a cache breakpoint, the automatic
// breakpoint, adaptive thinking that drops a mismatched block, the tool
// schema as input_schema, every tool result of the turn in one user
// message and the assistant's thinking first, byte for byte.
func TestRequestMapsSystemToolsThinkingAndCache(t *testing.T) {
	reasoning, _ := domain.NewJSONValue([]byte(`[` + thinkingBlock + `]`))
	arguments, _ := domain.NewJSONObject([]byte(`{"path":"a"}`))
	parameters, _ := domain.NewJSONObject([]byte(`{"type":"object","properties":{"path":{"type":"string"}}}`))
	req := domain.CompletionRequest{Model: "claude-sonnet-5-5", Messages: []domain.Message{
		{Role: domain.RoleSystem, Content: "Be brief."},
		{Role: domain.RoleUser, Content: "Read a and b"},
		{Role: domain.RoleAssistant, Content: "Reading.", Reasoning: reasoning, ToolCalls: []domain.ToolCall{{ID: "toolu_1", Name: "read", Arguments: arguments}, {ID: "toolu_2", Name: "read", Arguments: arguments}}},
		{Role: domain.RoleTool, ToolCallID: "toolu_1", Content: "one"},
		{Role: domain.RoleTool, ToolCallID: "toolu_2", Content: "two"},
	}, Tools: []domain.ToolDefinition{{Name: "read", Description: "Read a file", Parameters: parameters}}}
	var sent []byte
	var headers http.Header
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(stopEvents("end_turn")...))}, func(r *http.Request, raw []byte) {
		sent, headers = raw, r.Header.Clone()
	})
	if _, err := c.Stream(context.Background(), req, func(domain.Text) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if headers.Get("x-api-key") != "sk-ant-secret" || headers.Get("Authorization") != "" || headers.Get("anthropic-version") != apiVersion || headers.Get("anthropic-beta") != thinkingBeta || headers.Get("Accept") != "text/event-stream" {
		t.Fatalf("headers %v", headers)
	}
	body := decode(t, sent)
	want := decode(t, []byte(`{
		"model":"claude-sonnet-5-5","max_tokens":32000,"stream":true,
		"cache_control":{"type":"ephemeral"},
		"thinking":{"type":"adaptive","display":"summarized","block_binding":{"prefix_mismatch_behavior":"drop_block"}},
		"system":[{"type":"text","text":"Be brief.","cache_control":{"type":"ephemeral"}}],
		"messages":[
		  {"role":"user","content":[{"type":"text","text":"Read a and b"}]},
		  {"role":"assistant","content":[`+thinkingBlock+`,{"type":"text","text":"Reading."},{"type":"tool_use","id":"toolu_1","name":"read","input":{"path":"a"}},{"type":"tool_use","id":"toolu_2","name":"read","input":{"path":"a"}}]},
		  {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"one"},{"type":"tool_result","tool_use_id":"toolu_2","content":"two"}]}
		],
		"tools":[{"name":"read","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}]
	}`))
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("request\n%s\nwant\n%v", sent, want)
	}
}

// thinking: false sends no thinking configuration and no beta header, and a
// configured max_tokens replaces the default.
func TestRequestWithoutThinkingSendsNoThinkingConfig(t *testing.T) {
	off := false
	var sent []byte
	var headers http.Header
	c, _ := testClient(t, ClientConfig{Thinking: &off, MaxTokens: 4096}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(stopEvents("end_turn")...))}, func(r *http.Request, raw []byte) {
		sent, headers = raw, r.Header.Clone()
	})
	if _, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil }); err != nil {
		t.Fatal(err)
	}
	body := decode(t, sent)
	if _, present := body["thinking"]; present || headers.Get("anthropic-beta") != "" || body["max_tokens"] != float64(4096) {
		t.Fatalf("request %s headers %v", sent, headers)
	}
}

// A tool result needs the current tools, as the API requires.
func TestRequestRefusesToolResultsWithoutTools(t *testing.T) {
	arguments, _ := domain.NewJSONObject([]byte(`{}`))
	req := domain.CompletionRequest{Model: "claude-sonnet-5-5", Messages: []domain.Message{
		{Role: domain.RoleUser, Content: "Hi"},
		{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{ID: "toolu_1", Name: "read", Arguments: arguments}}},
		{Role: domain.RoleTool, ToolCallID: "toolu_1", Content: "one"},
	}}
	if _, err := mapRequest(req, 0, true); err == nil || !strings.Contains(err.Error(), "tool definitions") {
		t.Fatalf("err = %v", err)
	}
}

// stopEvents are the events of a stream that answers "ok" and stops.
func stopEvents(stop string) []string {
	return []string{
		`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-5-5","usage":{"input_tokens":10,"cache_creation_input_tokens":3,"cache_read_input_tokens":5,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"` + stop + `","stop_sequence":null},"usage":{"output_tokens":20}}`,
		`{"type":"message_stop"}`,
	}
}

// The stream claude-sonnet-5-5 would send for a turn that thinks, says a
// word and calls two tools: the thinking in pieces, then its signature, the
// text in two deltas, each tool's input as partial JSON.
var toolStream = []string{
	`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-sonnet-5-5","usage":{"input_tokens":10,"cache_creation_input_tokens":3,"cache_read_input_tokens":5,"output_tokens":1}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Comparing"}}`,
	`{"type":"ping"}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" the files"}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"CAQSqwc"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"hé"}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"🌍"}}`,
	`{"type":"content_block_stop","index":1}`,
	`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_a","name":"exec","input":{}}}`,
	`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"command\":"}}`,
	`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"pwd\"}"}}`,
	`{"type":"content_block_stop","index":2}`,
	`{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"toolu_b","name":"read","input":{}}}`,
	`{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"README.md\"}"}}`,
	`{"type":"content_block_stop","index":3}`,
	`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":20}}`,
	`{"type":"message_stop"}`,
}

func TestStreamTextThinkingAndParallelToolUse(t *testing.T) {
	body := &trackedStreamBody{Reader: strings.NewReader(sse(toolStream...))}
	c, calls := testClient(t, ClientConfig{}, 200, body, nil)
	var deltas []domain.Text
	got, err := c.Stream(context.Background(), simpleRequest(), func(text domain.Text) error { deltas = append(deltas, text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Content != "hé🌍" || len(deltas) != 2 || deltas[0] != "hé" || deltas[1] != "🌍" || got.FinishReason != "tool_calls" || got.Provider != "api.anthropic.com" || got.RequestBytes == 0 {
		t.Fatalf("result = %+v; deltas = %v", got, deltas)
	}
	if len(got.Message.ToolCalls) != 2 {
		t.Fatalf("calls = %v", got.Message.ToolCalls)
	}
	a, b := got.Message.ToolCalls[0], got.Message.ToolCalls[1]
	if a.ID != "toolu_a" || a.Name != "exec" || string(a.Arguments.Bytes()) != `{"command":"pwd"}` || b.ID != "toolu_b" || b.Name != "read" || string(b.Arguments.Bytes()) != `{"path":"README.md"}` {
		t.Fatalf("calls = %+v", got.Message.ToolCalls)
	}
	if string(got.Message.Reasoning.Bytes()) != `[`+thinkingBlock+`]` {
		t.Fatalf("reasoning = %s", got.Message.Reasoning.Bytes())
	}
	usage := got.Usage
	if usage == nil || usage.PromptTokens != 18 || usage.CachedTokens != 5 || usage.CacheWriteTokens != 3 || usage.CompletionTokens != 20 || usage.TotalTokens != 38 || usage.ReasoningTokens != 0 || usage.CostKnown {
		t.Fatalf("usage = %+v", usage)
	}
	if *calls != 1 || !body.closed {
		t.Fatalf("requests=%d closed=%v", *calls, body.closed)
	}
}

// The thinking block a stream returned goes back in the next request, first
// in its assistant message and unmodified, with the tool results that follow
// in one user message.
func TestStreamedThinkingGoesBackInTheNextRequest(t *testing.T) {
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(toolStream...))}, nil)
	first, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	parameters, _ := domain.NewJSONObject([]byte(`{"type":"object"}`))
	next := domain.CompletionRequest{Model: "claude-sonnet-5-5", Messages: []domain.Message{
		{Role: domain.RoleUser, Content: "Hi"},
		first.Message,
		{Role: domain.RoleTool, ToolCallID: "toolu_a", Content: "/home"},
		{Role: domain.RoleTool, ToolCallID: "toolu_b", Content: "# AXLR"},
	}, Tools: []domain.ToolDefinition{{Name: "exec", Parameters: parameters}, {Name: "read", Parameters: parameters}}}
	var sent []byte
	c, _ = testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(stopEvents("end_turn")...))}, func(_ *http.Request, raw []byte) { sent = raw })
	if _, err := c.Stream(context.Background(), next, func(domain.Text) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Messages []struct {
			Role    string            `json:"role"`
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(sent, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 3 || body.Messages[1].Role != "assistant" || len(body.Messages[1].Content) != 4 || string(body.Messages[1].Content[0]) != thinkingBlock || body.Messages[2].Role != "user" || len(body.Messages[2].Content) != 2 {
		t.Fatalf("messages %s", sent)
	}
}

// An answer the output limit cut short ends with the note, streamed too; a
// call cut short keeps its unreadable input as the malformed-arguments object.
func TestStreamMaxTokensAddsTheOutputLimitNote(t *testing.T) {
	events := []string{
		toolStream[0],
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Half an"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"max_tokens","stop_sequence":null},"usage":{"output_tokens":4096}}`,
		`{"type":"message_stop"}`,
	}
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(events...))}, nil)
	var streamed string
	got, err := c.Stream(context.Background(), simpleRequest(), func(text domain.Text) error { streamed += string(text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.FinishReason != "length" || got.Message.Content != "Half an\n\n"+provider.OutputLimitNote || streamed != string(got.Message.Content) {
		t.Fatalf("result = %+v streamed %q", got, streamed)
	}
	cut := []string{
		toolStream[0],
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_a","name":"write","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a\",\"content\":\"unfinis"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"max_tokens","stop_sequence":null},"usage":{"output_tokens":4096}}`,
		`{"type":"message_stop"}`,
	}
	c, _ = testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(cut...))}, nil)
	got, err = c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Message.ToolCalls) != 1 || !strings.Contains(string(got.Message.ToolCalls[0].Arguments.Bytes()), `"`+provider.MalformedArgumentsKey+`"`) || !strings.Contains(string(got.Message.ToolCalls[0].Arguments.Bytes()), `"cut_by_output_limit":true`) {
		t.Fatalf("calls = %+v", got.Message.ToolCalls)
	}
}

// A refusal says so in the message instead of returning an empty one.
func TestStreamRefusalKeepsTheExplanation(t *testing.T) {
	events := []string{
		toolStream[0],
		`{"type":"message_delta","delta":{"stop_reason":"refusal","stop_sequence":null,"stop_details":{"type":"refusal","category":"cyber","explanation":"The request asked for an exploit."}},"usage":{"output_tokens":0}}`,
		`{"type":"message_stop"}`,
	}
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(events...))}, nil)
	got, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.FinishReason != "refusal" || !strings.Contains(string(got.Message.Content), "declined") || !strings.Contains(string(got.Message.Content), "cyber") || !strings.Contains(string(got.Message.Content), "exploit") {
		t.Fatalf("result = %+v", got)
	}
}

// Redacted thinking goes back as it came, and an empty thinking block (the
// default display) keeps its signature.
func TestStreamKeepsRedactedAndEmptyThinking(t *testing.T) {
	events := []string{
		toolStream[0],
		`{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"EmwKAhgB"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"signature_delta","signature":"sig2"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"ok"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	}
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(events...))}, nil)
	got, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Message.Reasoning.Bytes()) != `[{"type":"redacted_thinking","data":"EmwKAhgB"},{"type":"thinking","thinking":"","signature":"sig2"}]` || got.FinishReason != "stop" {
		t.Fatalf("reasoning = %s finish %s", got.Message.Reasoning.Bytes(), got.FinishReason)
	}
}

func TestStreamRejectsIncompleteMalformedAndOversized(t *testing.T) {
	for name, wire := range map[string]string{
		"missing stop":    sse(toolStream[:len(toolStream)-1]...),
		"missing reason":  sse(toolStream[0], `{"type":"message_stop"}`),
		"malformed":       sse(toolStream[0]) + "event: x\ndata: {bad\n\n",
		"orphan delta":    sse(toolStream[0], `{"type":"content_block_delta","index":4,"delta":{"type":"text_delta","text":"x"}}`),
		"oversized event": sse(`{"type":"ping","pad":"` + strings.Repeat("x", 1024*1024) + `"}`),
		"invalid UTF-8":   sse(toolStream[0]) + "data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"" + string([]byte{0xff}) + "\"}}\n\n",
		"not assistant":   sse(`{"type":"message_start","message":{"role":"user"}}`),
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

// A mid-stream error event is the provider's error, classified by its type,
// named after the endpoint and without the key.
func TestStreamErrorEventIsAProviderError(t *testing.T) {
	events := []string{toolStream[0], `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded, key sk-ant-secret"}}`}
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(events...))}, nil)
	got, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
	var failed *provider.ProviderError
	if !errors.As(err, &failed) || failed.StatusCode != 529 || failed.Category != provider.CategoryRateLimited || failed.HintKind() != provider.HintRetryLater || strings.Contains(err.Error(), "sk-ant-secret") || !strings.HasPrefix(err.Error(), "model endpoint api.anthropic.com rate_limited (HTTP 529): Overloaded") {
		t.Fatalf("err = %v", err)
	}
	if got.RequestBytes == 0 || got.Message.Content != "" {
		t.Fatalf("sent = %+v", got)
	}
}

// HTTP errors keep the provider's message, classified, redacted, and with the
// hint the console shows: a bare "model: x" is Anthropic's unknown model.
func TestHTTPErrorsAreClassifiedWithoutTheKey(t *testing.T) {
	for _, tc := range []struct {
		status   int
		body     string
		category provider.ProviderErrorCategory
		hint     provider.ProviderHint
		message  string
	}{
		{401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key sk-ant-secret"}}`, provider.CategoryAuthentication, provider.HintCheckKey, "invalid x-api-key [redacted]"},
		{404, `{"type":"error","error":{"type":"not_found_error","message":"model: claude-nonexistent"}}`, provider.CategoryInvalidRequest, provider.HintChooseModel, "model: claude-nonexistent"},
		{429, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`, provider.CategoryRateLimited, provider.HintRetryLater, "slow down"},
		{529, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`, provider.CategoryRateLimited, provider.HintRetryLater, "Overloaded"},
		{400, `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: too large"}}`, provider.CategoryInvalidRequest, provider.HintNone, "max_tokens: too large"},
		{500, `<html>oops</html>`, provider.CategoryProviderFailure, provider.HintNone, ""},
	} {
		c, _ := testClient(t, ClientConfig{}, tc.status, io.NopCloser(strings.NewReader(tc.body)), nil)
		for name, call := range map[string]func() (domain.CompletionResult, error){
			"stream": func() (domain.CompletionResult, error) {
				return c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
			},
			"complete": func() (domain.CompletionResult, error) { return c.Complete(context.Background(), simpleRequest()) },
		} {
			c, _ = testClient(t, ClientConfig{}, tc.status, io.NopCloser(strings.NewReader(tc.body)), nil)
			got, err := call()
			var failed *provider.ProviderError
			if !errors.As(err, &failed) || failed.StatusCode != tc.status || failed.Category != tc.category || failed.HintKind() != tc.hint || failed.Message != tc.message || failed.Provider != "model endpoint api.anthropic.com" {
				t.Fatalf("%s HTTP %d: %v (%+v)", name, tc.status, err, failed)
			}
			if strings.Contains(err.Error(), "sk-ant-secret") || got.RequestBytes == 0 {
				t.Fatalf("%s HTTP %d leaked or lost the request: %v %+v", name, tc.status, err, got)
			}
			if failed.HintKind() == provider.HintCheckKey && !strings.Contains(failed.Hint(), "api_key_env") {
				t.Fatalf("hint %q names OpenRouter", failed.Hint())
			}
		}
	}
}

// A whole reply maps like a stream, its thinking kept byte for byte.
func TestCompleteMapsAWholeReply(t *testing.T) {
	reply := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-5-5","content":[` + thinkingBlock + `,{"type":"text","text":"Reading."},{"type":"tool_use","id":"toolu_1","name":"read","input":{"path":"a"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":900,"output_tokens":30}}`
	var sent []byte
	var headers http.Header
	c, _ := testClient(t, ClientConfig{}, 200, io.NopCloser(strings.NewReader(reply)), func(r *http.Request, raw []byte) { sent, headers = raw, r.Header.Clone() })
	got, err := c.Complete(context.Background(), simpleRequest())
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("Accept") == "text/event-stream" || strings.Contains(string(sent), `"stream"`) {
		t.Fatalf("whole reply asked for a stream: %s %v", sent, headers)
	}
	if got.Message.Content != "Reading." || len(got.Message.ToolCalls) != 1 || string(got.Message.Reasoning.Bytes()) != `[`+thinkingBlock+`]` || got.FinishReason != "tool_calls" || got.Usage.PromptTokens != 1000 || got.Usage.CachedTokens != 900 || got.Usage.CompletionTokens != 30 || got.RequestBytes != len(sent) {
		t.Fatalf("result = %+v reasoning %s", got, got.Message.Reasoning.Bytes())
	}
	c, _ = testClient(t, ClientConfig{}, 200, io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`)), nil)
	var failed *provider.ProviderError
	if _, err := c.Complete(context.Background(), simpleRequest()); !errors.As(err, &failed) || failed.Category != provider.CategoryInvalidRequest {
		t.Fatalf("error body with HTTP 200: %v", err)
	}
}

func TestNewChecksKeyAndEndpoint(t *testing.T) {
	if _, err := New(ClientConfig{}); err == nil || !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("no key: %v", err)
	}
	if _, err := New(ClientConfig{Endpoint: "http://127.0.0.1:8080/v1/messages"}); err != nil {
		t.Fatalf("loopback without key: %v", err)
	}
	if _, err := New(ClientConfig{APIKey: "k", Endpoint: "http://proxy.example.com/v1/messages"}); err == nil {
		t.Fatal("plain http off loopback accepted")
	}
	if _, err := New(ClientConfig{APIKey: "k\n", Endpoint: "https://proxy.example.com/v1/messages"}); err == nil {
		t.Fatal("key with a line break accepted")
	}
	if _, err := New(ClientConfig{APIKey: "k", MaxTokens: -1}); err == nil {
		t.Fatal("negative max_tokens accepted")
	}
	c, err := New(ClientConfig{APIKey: "k", Endpoint: "https://proxy.example.com/anthropic/v1/messages"})
	if err != nil || c.provider != "model endpoint proxy.example.com" || c.host != "proxy.example.com" {
		t.Fatalf("client %+v %v", c, err)
	}
}

// A cancelled context ends the stream with the context's error.
func TestStreamStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c, _ := testClient(t, ClientConfig{}, 200, &trackedStreamBody{Reader: strings.NewReader(sse(toolStream...))}, nil)
	_, err := c.Stream(ctx, simpleRequest(), func(domain.Text) error { cancel(); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
