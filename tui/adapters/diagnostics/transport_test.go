package diagnostics

import (
	"bytes"
	"context"
	"errors"
	"github.com/underpass-ai/AXLR/tui/application"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type traceEvents struct{ events []application.DiagnosticEvent }

func (s *traceEvents) Record(e application.DiagnosticEvent) error {
	s.events = append(s.events, e)
	return nil
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTransportCapturesTimelineWithoutChangingWireStream(t *testing.T) {
	trace := &traceEvents{}
	dir := filepath.Join(t.TempDir(), "payloads")
	payloads, _ := NewPayloadRecorder(dir, "sensitive-key")
	requestBody := `{"messages":[{"content":"sensitive-key"}],"tools":[{"name":"test"}]}`
	responseBody := ": PROCESSING\n\ndata: {\"choices\":[{\"delta\":{\"reasoning\":\"private reasoning\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"function\":{\"name\":\"tool\",\"arguments\":\"{}\"}}]}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"visible reply\"}}]}\n\ndata: [DONE]\n\n"
	transport := Transport{Trace: trace, Payloads: payloads, Next: transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != requestBody {
			t.Fatal("capture altered request")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(responseBody))}, nil
	})}
	req, _ := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", strings.NewReader(requestBody))
	req.Header.Set("Authorization", "Bearer sensitive-key")
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	small := make([]byte, 7)
	for {
		n, err := response.Body.Read(small)
		captured.Write(small[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if captured.String() != responseBody {
		t.Fatal("capture changed SSE response")
	}
	_ = response.Body.Close()
	counts := map[application.DiagnosticStage]int{}
	var id uint64
	for _, event := range trace.events {
		counts[event.Stage]++
		if event.Stage == application.DiagnosticRequestSent {
			id = event.RequestID
			if event.Messages != 1 || event.Tools != 1 || event.Bytes != len(requestBody) {
				t.Fatalf("request summary %+v", event)
			}
		}
	}
	for _, stage := range []application.DiagnosticStage{application.DiagnosticHeartbeat, application.DiagnosticReasoning, application.DiagnosticToolDelta, application.DiagnosticContent, application.DiagnosticWireDone} {
		if counts[stage] != 1 {
			t.Fatalf("missing timeline stage %s: %v", stage, counts)
		}
	}
	if id == 0 || counts[application.DiagnosticFrame] != 4 || counts[application.DiagnosticPayloadSaved] != 2 {
		t.Fatal(counts)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 2 {
		t.Fatal("payload captures missing")
	}
	for _, file := range files {
		data, _ := os.ReadFile(filepath.Join(dir, file.Name()))
		if bytes.Contains(data, []byte("sensitive-key")) || bytes.Contains(data, []byte("Authorization")) {
			t.Fatal("credential leaked")
		}
	}
}
func TestTransportIgnoresOtherEndpointsAndCapturesErrors(t *testing.T) {
	trace := &traceEvents{}
	boom := errors.New("offline")
	transport := Transport{Trace: trace, Next: transportFunc(func(*http.Request) (*http.Response, error) { return nil, boom })}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/api/v1/models", nil)
	if _, err := transport.RoundTrip(req); err != boom || len(trace.events) != 0 {
		t.Fatal("catalog changed")
	}
	req.URL.Host = "openrouter.ai"
	req.URL.Path = "/api/v1/chat/completions"
	req.ContentLength = -1
	if _, err := transport.RoundTrip(req); err != boom {
		t.Fatal(err)
	}
	for _, e := range trace.events {
		if e.Bytes < 0 {
			t.Fatal("negative measurement")
		}
	}
	if trace.events[len(trace.events)-1].ErrorClass != application.DiagnosticErrorProvider {
		t.Fatal("missing transport failure")
	}
}
func TestStreamTraceMalformedAndStructuredReasoningPreservesData(t *testing.T) {
	trace := &traceEvents{}
	body := &streamBody{next: io.NopCloser(strings.NewReader("data: bad json\n\ndata: {\"choices\":[{\"delta\":{\"reasoning_details\":[{\"text\":\"text\"},{\"summary\":\"sum\"}]}}]}\n\n")), trace: trace}
	_, _ = io.ReadAll(body)
	_ = body.Close()
	_ = body.Close()
	count := 0
	for _, e := range trace.events {
		if e.Stage == application.DiagnosticReasoning {
			count++
			if e.Bytes != 7 {
				t.Fatal(e.Bytes)
			}
		}
	}
	if count != 1 {
		t.Fatal(count)
	}
}

func TestStreamTraceRecognizesEverySSELineEndingAcrossReads(t *testing.T) {
	for _, ending := range []string{"\n", "\r", "\r\n"} {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(ending, "\r", "CR"), "\n", "LF"), func(t *testing.T) {
			trace := &traceEvents{}
			raw := ": processing" + ending + ending + `data: {"choices":[{"delta":{"content":"hello"}}]}` + ending + ending
			body := &streamBody{next: io.NopCloser(strings.NewReader(raw)), trace: trace}
			var result bytes.Buffer
			one := make([]byte, 1)
			for {
				n, err := body.Read(one)
				result.Write(one[:n])
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			_ = body.Close()
			if result.String() != raw {
				t.Fatal("wire modified")
			}
			counts := map[application.DiagnosticStage]int{}
			for _, e := range trace.events {
				counts[e.Stage]++
			}
			if counts[application.DiagnosticContent] != 1 || counts[application.DiagnosticHeartbeat] != 1 || counts[application.DiagnosticFrame] != 1 {
				t.Fatal(counts)
			}
		})
	}
}

type countClosedBody struct {
	io.Reader
	closed int
}

func (b *countClosedBody) Close() error { b.closed++; return nil }
func TestStreamBodyClosesUnderlyingBodyOnce(t *testing.T) {
	next := &countClosedBody{Reader: strings.NewReader("")}
	body := &streamBody{next: next}
	_ = body.Close()
	_ = body.Close()
	if next.closed != 1 {
		t.Fatal(next.closed)
	}
}
func TestTransportReportsUnavailableAndOversizedRequestCaptures(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		trace := &traceEvents{}
		recorder, _ := NewPayloadRecorder(filepath.Join(t.TempDir(), "payloads"))
		transport := Transport{Trace: trace, Payloads: recorder, Next: transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
		req, _ := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", strings.NewReader(strings.Repeat("x", payloadLimit+1)))
		if !oversized {
			req.GetBody = nil
		}
		_, _ = transport.RoundTrip(req)
		count := 0
		for _, e := range trace.events {
			if e.Stage == application.DiagnosticPayloadFailed {
				count++
			}
		}
		if count != 2 {
			t.Fatal(trace.events)
		}
	}
}

func TestTransportCapturesUnreadProviderErrorPayloadOnClose(t *testing.T) {
	trace := &traceEvents{}
	dir := filepath.Join(t.TempDir(), "payloads")
	recorder, _ := NewPayloadRecorder(dir)
	transport := Transport{Trace: trace, Payloads: recorder, Next: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"error":"quota"}`))}, nil
	})}
	req, _ := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", strings.NewReader(`{}`))
	resp, _ := transport.RoundTrip(req)
	_ = resp.Body.Close()
	failed := 0
	for _, e := range trace.events {
		if e.Stage == application.DiagnosticPayloadFailed {
			failed++
		}
	}
	files, _ := os.ReadDir(dir)
	if failed != 0 || len(files) != 2 {
		t.Fatalf("files=%v events=%v", files, trace.events)
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), "response.json") {
			data, err := os.ReadFile(filepath.Join(dir, file.Name()))
			if err != nil || string(data) != `{"error":"quota"}` {
				t.Fatalf("payload=%s err=%v", data, err)
			}
		}
	}
}

func TestTransportCapturesModelsGETRequestAndJSONResponseWithSpans(t *testing.T) {
	trace := &traceEvents{}
	dir := filepath.Join(t.TempDir(), "payloads")
	recorder, _ := NewPayloadRecorder(dir)
	raw := `{"data":[{"id":"test/model"}]}`
	transport := Transport{Trace: trace, Payloads: recorder, Next: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Body != nil || application.CurrentDiagnosticSpan(r.Context()) == 0 {
			t.Fatal("empty GET body or missing HTTP context")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})}
	req, _ := http.NewRequest(http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || string(data) != raw {
		t.Fatalf("data=%s err=%v", data, err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 2 {
		t.Fatal(files)
	}
	var starts, ends, saved int
	for _, event := range trace.events {
		if event.Action == application.DiagnosticActionHTTP {
			if event.Stage == application.DiagnosticActionStart {
				starts++
			}
			if event.Stage == application.DiagnosticActionEnd {
				ends++
				if event.ErrorClass != "" {
					t.Fatal(event)
				}
			}
		}
		if event.Stage == application.DiagnosticPayloadSaved {
			saved++
		}
		if event.Stage == application.DiagnosticFrame || event.Stage == application.DiagnosticPayloadFailed {
			t.Fatal(event)
		}
	}
	if starts != 1 || ends != 1 || saved != 2 {
		t.Fatal(trace.events)
	}
	for _, file := range files {
		body, _ := os.ReadFile(filepath.Join(dir, file.Name()))
		if !strings.HasSuffix(file.Name(), ".json") {
			t.Fatal(file.Name())
		}
		if strings.HasSuffix(file.Name(), "request.json") && len(body) != 0 {
			t.Fatal("GET request capture was not empty")
		}
		if strings.HasSuffix(file.Name(), "response.json") && string(body) != raw {
			t.Fatal("catalog payload changed")
		}
	}
}

type blockingResponseBody struct {
	closed chan struct{}
	once   sync.Once
}

func (b *blockingResponseBody) Read([]byte) (int, error) { <-b.closed; return 0, io.ErrClosedPipe }
func (b *blockingResponseBody) Close() error             { b.once.Do(func() { close(b.closed) }); return nil }

func TestUnreadErrorDrainIsBoundedAndCancelledWithoutChangingReadBytes(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		trace := &traceEvents{}
		recorder, _ := NewPayloadRecorder(filepath.Join(t.TempDir(), "payloads"))
		ctx, cancel := context.WithCancel(context.Background())
		body := &blockingResponseBody{closed: make(chan struct{})}
		transport := Transport{Trace: trace, Payloads: recorder, Next: transportFunc(func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 503, Body: body}, nil })}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
		resp, _ := transport.RoundTrip(req)
		if cancelled {
			cancel()
		}
		started := time.Now()
		_ = resp.Body.Close()
		cancel()
		if time.Since(started) > 1500*time.Millisecond {
			t.Fatal("error drain exceeded time budget")
		}
		// The drain ends the wire and the HTTP exchange with its class; the
		// empty capture of what arrived is still written, so it is saved.
		failed, ended, wireDone := 0, 0, 0
		want := application.DiagnosticErrorTimeout
		if cancelled {
			want = application.DiagnosticErrorCancelled
		}
		for _, event := range trace.events {
			if event.Stage == application.DiagnosticPayloadFailed {
				failed++
			}
			if event.Stage == application.DiagnosticWireDone {
				wireDone++
				if event.ErrorClass != want {
					t.Fatal(event)
				}
			}
			if event.Action == application.DiagnosticActionHTTP && event.Stage == application.DiagnosticActionEnd {
				ended++
				if event.ErrorClass != want {
					t.Fatal(event)
				}
			}
		}
		if failed != 0 || ended != 1 || wireDone != 1 {
			t.Fatal(trace.events)
		}
	}
}

func TestErrorDrainCapturesRemainingJSONAfterClientReadAndBoundsOversize(t *testing.T) {
	// Reading 8 MiB took longer than the 500 ms bound on a slow macOS
	// runner, which then saved a truncated capture; this test is about
	// the size bound, not the time bound.
	previous := errorDrainTimeout
	errorDrainTimeout = time.Minute
	t.Cleanup(func() { errorDrainTimeout = previous })
	for _, oversized := range []bool{false, true} {
		trace := &traceEvents{}
		dir := filepath.Join(t.TempDir(), "payloads")
		recorder, _ := NewPayloadRecorder(dir)
		raw := `{"error":"provider failed"}`
		if oversized {
			raw = strings.Repeat("x", payloadLimit+128)
		}
		transport := Transport{Trace: trace, Payloads: recorder, Next: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(raw))}, nil
		})}
		req, _ := http.NewRequest(http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
		resp, _ := transport.RoundTrip(req)
		prefix := make([]byte, 3)
		if _, err := io.ReadFull(resp.Body, prefix); err != nil || string(prefix) != raw[:3] {
			t.Fatal("client read was changed")
		}
		_ = resp.Body.Close()
		failed := 0
		for _, event := range trace.events {
			if event.Stage == application.DiagnosticPayloadFailed {
				failed++
			}
		}
		files, _ := os.ReadDir(dir)
		if !oversized {
			if failed != 0 || len(files) != 2 {
				t.Fatal(trace.events)
			}
			for _, file := range files {
				if strings.HasSuffix(file.Name(), "response.json") {
					data, _ := os.ReadFile(filepath.Join(dir, file.Name()))
					if string(data) != raw {
						t.Fatal("remaining error body not captured")
					}
				}
			}
		} else if failed != 1 {
			t.Fatal(trace.events)
		}
	}
}

// A stream the person cancels is still captured: the trace marks the wire
// cancelled and the capture saved, not a storage failure for a file that
// exists.
func TestCancelledStreamCaptureIsSavedAndWireIsCancelled(t *testing.T) {
	trace := &traceEvents{}
	dir := filepath.Join(t.TempDir(), "payloads")
	recorder, _ := NewPayloadRecorder(dir)
	wire := "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" answer\"}}]}\n\n"
	transport := Transport{Trace: trace, Payloads: recorder, Next: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", strings.NewReader(`{}`))
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	first := make([]byte, 20)
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		t.Fatal(err)
	}
	cancel()
	_ = resp.Body.Close()
	var saved, failed, failedSpans int
	var wireDone application.DiagnosticErrorClass
	for _, event := range trace.events {
		switch event.Stage {
		case application.DiagnosticPayloadSaved:
			saved++
		case application.DiagnosticPayloadFailed:
			failed++
		case application.DiagnosticWireDone:
			wireDone = event.ErrorClass
		}
		if event.Action == application.DiagnosticActionPayload && event.Stage == application.DiagnosticActionEnd && event.ErrorClass != "" {
			failedSpans++
		}
	}
	if saved != 2 || failed != 0 || failedSpans != 0 || wireDone != application.DiagnosticErrorCancelled {
		t.Fatalf("saved=%d failed=%d failed capture spans=%d wire_done=%q", saved, failed, failedSpans, wireDone)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*-response.sse"))
	if len(matches) != 1 {
		t.Fatalf("response capture missing: %v", matches)
	}
	if data, _ := os.ReadFile(matches[0]); string(data) != wire[:20] {
		t.Fatalf("capture=%q", data)
	}
}

func TestTransportIdentifiesProviderEndpointsWithoutRawURLs(t *testing.T) {
	for _, tc := range []struct {
		path     string
		endpoint application.DiagnosticEndpoint
	}{
		{"/api/v1/chat/completions", application.DiagnosticEndpointChat},
		{"/api/v1/models", application.DiagnosticEndpointModels},
		{"/api/v1/generation?id=private-generation", application.DiagnosticEndpointOther},
	} {
		trace := &traceEvents{}
		transport := Transport{Trace: trace, Next: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
		})}
		req, _ := http.NewRequest(http.MethodGet, "https://openrouter.ai"+tc.path, nil)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		found := map[application.DiagnosticStage]bool{}
		for _, event := range trace.events {
			if event.Endpoint != tc.endpoint {
				t.Fatalf("path=%s event=%+v", tc.path, event)
			}
			found[event.Stage] = true
		}
		for _, stage := range []application.DiagnosticStage{application.DiagnosticActionStart, application.DiagnosticActionEnd, application.DiagnosticRequestSent, application.DiagnosticProviderHeaders, application.DiagnosticWireBytes, application.DiagnosticWireDone} {
			if !found[stage] {
				t.Fatalf("missing%s for%s", stage, tc.path)
			}
		}
	}
}
