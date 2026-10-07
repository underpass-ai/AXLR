package diagnostics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestTransportTracesConfiguredLocalEndpointAndReportsActivity(t *testing.T) {
	wire := "data: {\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: [DONE]\n\n"
	trace := &traceEvents{}
	transport := Transport{Trace: trace, Endpoints: []string{"http://127.0.0.1:8080/v1"}, Next: transportFunc(func(*http.Request) (*http.Response, error) {
		// llama.cpp may omit the content type; the chat path still means SSE.
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire))}, nil
	})}
	var phases []domain.ProviderPhase
	ctx := application.WithProviderActivity(context.Background(), func(phase domain.ProviderPhase) { phases = append(phases, phase) })
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:8080/v1/chat/completions", strings.NewReader(`{"messages":[]}`))
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	sent := false
	for _, event := range trace.events {
		if event.Stage == application.DiagnosticRequestSent {
			sent = event.Endpoint == application.DiagnosticEndpointChat
		}
	}
	if !sent {
		t.Fatalf("local chat request not traced: %+v", trace.events)
	}
	if len(phases) < 2 || phases[0] != domain.ProviderReasoning || phases[len(phases)-1] != domain.ProviderContent {
		t.Fatalf("provider activity = %v", phases)
	}
}

func TestTransportIgnoresUnconfiguredLocalServers(t *testing.T) {
	for _, target := range []string{
		"http://127.0.0.1:8081/v1/chat/completions",
		"https://127.0.0.1:8080/v1/chat/completions",
		"http://127.0.0.1:8080/other/chat/completions",
		"http://127.0.0.1:8080/v1",
	} {
		trace := &traceEvents{}
		transport := Transport{Trace: trace, Endpoints: []string{"http://127.0.0.1:8080/v1/"}, Next: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
		})}
		req, _ := http.NewRequest(http.MethodGet, target, nil)
		resp, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if len(trace.events) != 0 {
			t.Fatalf("%s traced: %+v", target, trace.events)
		}
	}
}
