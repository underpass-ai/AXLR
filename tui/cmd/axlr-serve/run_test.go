package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/adapters/openrouter"
	"github.com/underpass-ai/AXLR/domain"
)

const (
	deltaChunk = `{"choices":[{"index":0,"delta":{"role":"assistant","content":"x"},"finish_reason":null}]}`
	stopChunk  = `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`
)

func streamThrough(t *testing.T, client *http.Client, handler http.HandlerFunc) (int, error) {
	t.Helper()
	server := httptest.NewServer(handler)
	defer server.Close()
	model, err := openrouter.New(openrouter.ClientConfig{APIKey: "k", Endpoint: server.URL + "/v1/chat/completions", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	deltas := 0
	_, err = model.Stream(context.Background(), domain.CompletionRequest{Model: "m/m", Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}}}, func(domain.Text) error {
		deltas++
		return nil
	})
	return deltas, err
}

// The bound covers the wait for response headers, not an answer that keeps
// streaming; scaled down from 60 s to 300 ms.
func TestModelHTTPClientKeepsAnActiveStreamPastItsBound(t *testing.T) {
	deltas, err := streamThrough(t, modelHTTPClient(300*time.Millisecond), func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for i := 0; i < 20; i++ {
			fmt.Fprintf(w, "data: %s\n\n", deltaChunk)
			flusher.Flush()
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", stopChunk)
		flusher.Flush()
	})
	if err != nil || deltas != 20 {
		t.Fatalf("a 1 s stream was cut after %d deltas: %v", deltas, err)
	}
}

func TestModelHTTPClientBoundsTheWaitForHeaders(t *testing.T) {
	start := time.Now()
	_, err := streamThrough(t, modelHTTPClient(300*time.Millisecond), func(w http.ResponseWriter, r *http.Request) {
		// Reading the body lets the server notice the client hanging up.
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	if err == nil {
		t.Fatal("a provider that never answers did not fail")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("waited %v for headers", elapsed)
	}
}
