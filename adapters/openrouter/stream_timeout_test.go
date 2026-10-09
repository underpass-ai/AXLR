package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/domain"
)

const testStreamInactivityTimeout = 40 * time.Millisecond

func timeoutStreamClient(t *testing.T, transport roundTripFunc) *Client {
	t.Helper()
	client, err := New(ClientConfig{
		APIKey:                  "secret-token",
		HTTPClient:              &http.Client{Transport: transport},
		StreamInactivityTimeout: testStreamInactivityTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func assertStreamTimedOut(t *testing.T, err error) {
	t.Helper()
	var timeout *StreamTimeoutError
	if !errors.As(err, &timeout) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected typed stream timeout, got %T: %v", err, err)
	}
	if strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "sensitive-provider-text") {
		t.Fatalf("timeout leaked request or provider data: %v", err)
	}
}

func awaitStreamError(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("stream remained blocked")
		return nil
	}
}

func TestStreamTimesOutWaitingForResponseHeaders(t *testing.T) {
	client := timeoutStreamClient(t, func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	done := make(chan error, 1)
	go func() {
		_, err := client.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
		done <- err
	}()
	assertStreamTimedOut(t, awaitStreamError(t, done))
}

func TestStreamTimesOutAfterDataDespiteHeartbeats(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	client := timeoutStreamClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: reader, Header: make(http.Header)}, nil
	})
	done := make(chan error, 1)
	texts := make(chan domain.Text, 1)
	go func() {
		_, err := client.Stream(context.Background(), simpleCompletionRequest(), func(text domain.Text) error {
			texts <- text
			return nil
		})
		done <- err
	}()
	if _, err := io.WriteString(writer, streamEvents(textChunk)); err != nil {
		t.Fatal(err)
	}
	select {
	case text := <-texts:
		if text != "hé🌍" {
			t.Fatalf("text = %q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("initial data frame not delivered")
	}
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := io.WriteString(writer, ": sensitive-provider-text\n\n"); err != nil {
				return
			}
		}
	}()
	assertStreamTimedOut(t, awaitStreamError(t, done))
}

func TestStreamTimesOutAfterFinishWithoutDone(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	client := timeoutStreamClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: reader, Header: make(http.Header)}, nil
	})
	done := make(chan error, 1)
	go func() {
		_, err := client.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
		done <- err
	}()
	if _, err := io.WriteString(writer, streamEvents(textChunk, stopChunk)); err != nil {
		t.Fatal(err)
	}
	assertStreamTimedOut(t, awaitStreamError(t, done))
	closed := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, ": after timeout\n\n")
		closed <- err
	}()
	select {
	case err := <-closed:
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("response body was not closed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("response body remained open after timeout")
	}
}

func TestStreamCancellationPrecedesInactivityTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer writer.Close()
	client := timeoutStreamClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: reader, Header: make(http.Header)}, nil
	})
	done := make(chan error, 1)
	go func() {
		_, err := client.Stream(ctx, simpleCompletionRequest(), func(domain.Text) error { return nil })
		done <- err
	}()
	if _, err := io.WriteString(writer, ": heartbeat\n\n"); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := awaitStreamError(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestStreamMaximumDurationBoundsNonTextFrames(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := New(ClientConfig{
		APIKey: "secret-token",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: reader, Header: make(http.Header)}, nil
		})},
		StreamInactivityTimeout: 80 * time.Millisecond,
		StreamMaxDuration:       120 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := client.Stream(ctx, simpleCompletionRequest(), func(domain.Text) error { return nil })
		done <- err
	}()
	var frames atomic.Int32
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := io.WriteString(writer, streamEvents(`{"choices":[{"index":0,"delta":{},"finish_reason":null}]}`)); err != nil {
				return
			}
			frames.Add(1)
		}
	}()
	err = awaitStreamError(t, done)
	assertStreamTimedOut(t, err)
	if !strings.Contains(err.Error(), "maximum duration") || frames.Load() < 3 {
		t.Fatalf("err=%v, frames=%d; expected duration cap after continued frames", err, frames.Load())
	}
}

// A local model configured with "stream": false is served by Complete: the
// stream's maximum duration bounds it too, whether the server never answers
// or never finishes the body, so a hung server does not hold the turn.
func TestCompleteTimesOutAfterMaximumDuration(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	for name, transport := range map[string]roundTripFunc{
		"no headers": func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		},
		"endless body": func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: reader, Header: make(http.Header)}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			client, err := New(ClientConfig{Endpoint: "http://127.0.0.1:8000/v1/chat/completions", HTTPClient: &http.Client{Transport: transport}, StreamMaxDuration: 50 * time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				_, err := client.Complete(context.Background(), simpleCompletionRequest())
				done <- err
			}()
			err = awaitStreamError(t, done)
			assertStreamTimedOut(t, err)
			if err.Error() != "model endpoint 127.0.0.1:8000 stream maximum duration exceeded" {
				t.Fatalf("error = %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	client, err := New(ClientConfig{APIKey: "k", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		cancel()
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}})
	if err != nil {
		t.Fatal(err)
	}
	var timeout *StreamTimeoutError
	if _, err := client.Complete(ctx, simpleCompletionRequest()); !errors.Is(err, context.Canceled) || errors.As(err, &timeout) {
		t.Fatalf("cancelled completion = %T %v", err, err)
	}
}
