package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/underpass-ai/AXLR/application"
	"github.com/underpass-ai/AXLR/domain"
)

var _ application.ModelStreamPort = (*Client)(nil)

func streamEvents(chunks ...string) string {
	return "data: " + strings.Join(chunks, "\n\ndata: ") + "\n\n"
}

const textChunk = `{"choices":[{"index":0,"delta":{"role":"assistant","content":"hé🌍"},"finish_reason":null}]}`
const stopChunk = `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`

type trackedStreamBody struct {
	io.Reader
	closed bool
}

func (b *trackedStreamBody) Close() error { b.closed = true; return nil }

func streamClient(t *testing.T, body io.ReadCloser, status int) (*Client, *int) {
	t.Helper()
	calls := new(int)
	c, err := New(ClientConfig{APIKey: "secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		*calls++
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, raw, `{"model":"openai/gpt-4o","messages":[{"role":"user","content":"Hi"}],"stream":true}`)
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing authorization")
		}
		return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	return c, calls
}

func TestStreamTextAndParallelCalls(t *testing.T) {
	wire := streamEvents(textChunk,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"read","arguments":"{\"path\":"}},{"index":0,"id":"call_a","type":"function","function":{"name":"exec","arguments":"{\"command\":\""}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"pwd\"}"}},{"index":1,"function":{"arguments":"\"README.md\"}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`, "[DONE]")
	body := &trackedStreamBody{Reader: iotest.OneByteReader(strings.NewReader(wire))}
	c, calls := streamClient(t, body, 200)
	var deltas []domain.Text
	got, err := c.Stream(context.Background(), simpleCompletionRequest(), func(text domain.Text) error { deltas = append(deltas, text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Content != "hé🌍" || len(deltas) != 1 || deltas[0] != "hé🌍" || got.FinishReason != "tool_calls" || got.Usage == nil || got.Usage.TotalTokens != 7 {
		t.Fatalf("result = %+v; deltas = %v", got, deltas)
	}
	if len(got.Message.ToolCalls) != 2 {
		t.Fatalf("calls = %v", got.Message.ToolCalls)
	}
	a, b := got.Message.ToolCalls[0], got.Message.ToolCalls[1]
	if a.ID != "call_a" || a.Name != "exec" || string(a.Arguments.Bytes()) != `{"command":"pwd"}` || b.ID != "call_b" || b.Name != "read" || string(b.Arguments.Bytes()) != `{"path":"README.md"}` {
		t.Fatalf("calls = %+v", got.Message.ToolCalls)
	}
	if *calls != 1 || !body.closed {
		t.Fatalf("requests=%d closed=%v", *calls, body.closed)
	}
}

func TestStreamIgnoresDeltasAfterFinish(t *testing.T) {
	wire := streamEvents(textChunk, stopChunk,
		`{"choices":[{"index":0,"delta":{"content":"late","tool_calls":[{"index":0,"id":"ignored","type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`, "[DONE]")
	body := &trackedStreamBody{Reader: strings.NewReader(wire)}
	c, _ := streamClient(t, body, 200)
	var deltas []domain.Text
	got, err := c.Stream(context.Background(), simpleCompletionRequest(), func(text domain.Text) error {
		deltas = append(deltas, text)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Content != "hé🌍" || len(got.Message.ToolCalls) != 0 || got.FinishReason != "stop" || got.Usage == nil || got.Usage.TotalTokens != 7 || len(deltas) != 1 || deltas[0] != "hé🌍" {
		t.Fatalf("result = %+v; deltas = %v", got, deltas)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
}

func TestStreamRejectsIncompleteMalformedAndOversized(t *testing.T) {
	for name, wire := range map[string]string{
		"missing finish": streamEvents(textChunk, "[DONE]"),
		"missing done":   streamEvents(textChunk, stopChunk),
		"malformed":      streamEvents("{bad", "[DONE]"),
		"oversized":      streamEvents(strings.Repeat("x", 1024*1024)),
		"aggregate":      strings.Repeat(streamEvents(`{"choices":[],"padding":"`+strings.Repeat("x", 512*1024)+`"}`), 17),
		"missing ID":     streamEvents(`{"choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"read","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, "[DONE]"),
		"missing name":   streamEvents(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","type":"function","function":{"arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, "[DONE]"),
		"invalid UTF8":   streamEvents(`{"choices":[{"delta":{"content":"` + string([]byte{0xff}) + `"}}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			body := &trackedStreamBody{Reader: strings.NewReader(wire)}
			c, calls := streamClient(t, body, 200)
			if _, err := c.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil }); err == nil {
				t.Fatal("invalid stream accepted")
			}
			if *calls != 1 || !body.closed {
				t.Fatalf("requests=%d closed=%v", *calls, body.closed)
			}
		})
	}
}

// The id and name of a tool call arrive once in OpenAI's stream; a provider
// that repeats them in every chunk must not get "call_1call_1" back. Only
// the arguments are fragments.
func TestStreamToolCallRepeatedIDAndNameAreNotConcatenated(t *testing.T) {
	wire := streamEvents(
		`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read","arguments":"{\"path\":"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read","arguments":"\"a\"}"}}]},"finish_reason":"tool_calls"}]}`,
		"[DONE]")
	body := &trackedStreamBody{Reader: strings.NewReader(wire)}
	c, _ := streamClient(t, body, 200)
	got, err := c.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Message.ToolCalls) != 1 {
		t.Fatalf("calls = %+v", got.Message.ToolCalls)
	}
	call := got.Message.ToolCalls[0]
	if call.ID != "call_1" || call.Name != "read" || string(call.Arguments.Bytes()) != `{"path":"a"}` {
		t.Fatalf("id=%q name=%q arguments=%s", call.ID, call.Name, call.Arguments.Bytes())
	}
}

// A tool call cut by the output limit completes the stream: the call keeps
// a valid object that names the cut, for the turn to answer with a tool
// error, instead of the whole operation failing on invalid JSON.
func TestStreamKeepsACutToolCallForAToolError(t *testing.T) {
	wire := streamEvents(
		`{"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"local_write","arguments":"{\"path\":\"a.go\","}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"content\":\"package"}}]},"finish_reason":"length"}]}`,
		"[DONE]")
	body := &trackedStreamBody{Reader: strings.NewReader(wire)}
	c, _ := streamClient(t, body, 200)
	got, err := c.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	want := `{"axlr_malformed_arguments":{"error":"unexpected end of JSON input","raw_prefix":"{\"path\":\"a.go\",\"content\":\"package","cut_by_output_limit":true}}`
	if got.FinishReason != "length" || len(got.Message.ToolCalls) != 1 || got.Message.ToolCalls[0].Name != "local_write" || string(got.Message.ToolCalls[0].Arguments.Bytes()) != want {
		t.Fatalf("result = %+v", got)
	}
}

// The note on an answer cut by the output limit is streamed like its text,
// so the live view shows what the session keeps.
func TestStreamMarksAnAnswerCutByTheOutputLimit(t *testing.T) {
	wire := streamEvents(`{"choices":[{"index":0,"delta":{"role":"assistant","content":"The three steps are: 1. Back up the"},"finish_reason":"length"}]}`, "[DONE]")
	body := &trackedStreamBody{Reader: strings.NewReader(wire)}
	c, _ := streamClient(t, body, 200)
	var streamed strings.Builder
	got, err := c.Stream(context.Background(), simpleCompletionRequest(), func(text domain.Text) error {
		streamed.WriteString(string(text))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "The three steps are: 1. Back up the\n\n" + outputLimitNote
	if string(got.Message.Content) != want || streamed.String() != want || got.FinishReason != "length" {
		t.Fatalf("content = %q, streamed = %q, finish = %q", got.Message.Content, streamed.String(), got.FinishReason)
	}
}

func TestStreamProviderErrors(t *testing.T) {
	for _, status := range []int{401, 402, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			for _, mid := range []bool{false, true} {
				wire := "sensitive"
				httpStatus := status
				if mid {
					wire = streamEvents(textChunk, `{"error":{"code":`+strconv.Itoa(status)+`,"message":"sensitive"}}`)
					httpStatus = 200
				}
				body := &trackedStreamBody{Reader: strings.NewReader(wire)}
				c, calls := streamClient(t, body, httpStatus)
				_, err := c.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
				var pe *ProviderError
				if !errors.As(err, &pe) || pe.StatusCode != status || pe.Category != classifyProviderError(status).Category || strings.Contains(err.Error(), "sensitive") || *calls != 1 || !body.closed {
					t.Fatalf("err=%v requests=%d closed=%v", err, *calls, body.closed)
				}
			}
		})
	}
}

func TestStreamStopsOnCallbackError(t *testing.T) {
	body := &trackedStreamBody{Reader: strings.NewReader(streamEvents(textChunk, textChunk, stopChunk, "[DONE]"))}
	c, calls := streamClient(t, body, 200)
	want := errors.New("display stopped")
	emissions := 0
	_, err := c.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { emissions++; return want })
	if err != want || emissions != 1 || *calls != 1 || !body.closed {
		t.Fatalf("err=%v emissions=%d closed=%v", err, emissions, body.closed)
	}
}

func TestStreamCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, calls := streamClient(t, io.NopCloser(strings.NewReader("")), 200)
	if _, err := c.Stream(ctx, simpleCompletionRequest(), func(domain.Text) error { return nil }); !errors.Is(err, context.Canceled) || *calls != 0 {
		t.Fatalf("err=%v calls=%d", err, *calls)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer writer.Close()
	c, calls = streamClient(t, reader, 200)
	done := make(chan error, 1)
	go func() {
		_, err := c.Stream(ctx, simpleCompletionRequest(), func(domain.Text) error { return nil })
		done <- err
	}()
	// Receipt proves Stream is blocked reading a live response body.
	if _, err := writer.Write([]byte(": keepalive\n\n")); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not unblock read")
	}
}
