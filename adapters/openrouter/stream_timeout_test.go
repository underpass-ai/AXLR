package openrouter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
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

// manualTimer is a streamTimer that fires only when the test calls fire.
type manualTimer struct {
	duration time.Duration
	fire     func()
	resets   atomic.Int32
	reset    chan struct{}
}

func (m *manualTimer) Stop() bool { return true }

func (m *manualTimer) Reset(time.Duration) bool {
	m.resets.Add(1)
	select {
	case m.reset <- struct{}{}:
	default:
	}
	return true
}

// manualClock hands out manualTimers in the order a stream starts them and
// signals each Reset on reset.
type manualClock struct {
	mu     sync.Mutex
	timers []*manualTimer
	reset  chan struct{}
}

func (c *manualClock) afterFunc(d time.Duration, f func()) streamTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := &manualTimer{duration: d, fire: f, reset: c.reset}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *manualClock) timer(i int) *manualTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.timers[i]
}

// Frames that carry no text still reset the inactivity timer, and never the
// maximum duration, which cuts the stream while they keep arriving.
func TestStreamMaximumDurationBoundsNonTextFrames(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
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
	clock := &manualClock{reset: make(chan struct{}, 1)}
	client.afterFunc = clock.afterFunc
	done := make(chan error, 1)
	go func() {
		_, err := client.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
		done <- err
	}()
	go func() {
		for {
			if _, err := io.WriteString(writer, streamEvents(`{"choices":[{"index":0,"delta":{},"finish_reason":null}]}`)); err != nil {
				return
			}
		}
	}()
	for resets := int32(0); resets < 3; resets = clock.timer(0).resets.Load() {
		select {
		case <-clock.reset:
		case <-time.After(time.Second):
			t.Fatal("non-text frames did not reset the inactivity timer")
		}
	}
	inactivity, maximum := clock.timer(0), clock.timer(1)
	if inactivity.duration != 80*time.Millisecond || maximum.duration != 120*time.Millisecond {
		t.Fatalf("timers = %v, %v", inactivity.duration, maximum.duration)
	}
	maximum.fire()
	err = awaitStreamError(t, done)
	assertStreamTimedOut(t, err)
	if !strings.Contains(err.Error(), "maximum duration") || maximum.resets.Load() != 0 {
		t.Fatalf("err=%v, maximum duration resets=%d; expected an unextended duration cap", err, maximum.resets.Load())
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

func TestStreamTimeoutErrorTellsItsKindAndProvider(t *testing.T) {
	cut := NewStreamTimeoutError("", true)
	if !cut.MaximumDuration() || cut.Provider() != "OpenRouter" || cut.Error() != "OpenRouter stream maximum duration exceeded" {
		t.Fatalf("%v %q %q", cut.MaximumDuration(), cut.Provider(), cut.Error())
	}
	idle := NewStreamTimeoutError("llama", false)
	if idle.MaximumDuration() || idle.Provider() != "llama" || idle.Error() != "llama stream inactivity timeout" {
		t.Fatalf("%v %q %q", idle.MaximumDuration(), idle.Provider(), idle.Error())
	}
}

// A model's own inactivity timeout replaces the client's on OpenRouter, and
// is never sent (issue #167).
func TestModelInactivityTimeoutReplacesTheClients(t *testing.T) {
	var sent []byte
	client, err := New(ClientConfig{
		APIKey: "secret-token",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			sent, _ = io.ReadAll(req.Body)
			<-req.Context().Done()
			return nil, req.Context().Err()
		})},
		StreamInactivityTimeout: time.Hour,
		Models:                  map[string]ModelOptions{"openai/gpt-4o": {StreamInactivityTimeout: testStreamInactivityTimeout}},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := client.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
		done <- err
	}()
	assertStreamTimedOut(t, awaitStreamError(t, done))
	if bytes.Contains(sent, []byte("nactivity")) || bytes.Contains(sent, []byte("timeout")) {
		t.Fatalf("the timeout reached the request: %s", sent)
	}
}

// Until 22 October 2026 a stream was cut after one silent minute.
func TestDefaultInactivityTimeoutIsTheStreamsLength(t *testing.T) {
	client, err := New(ClientConfig{APIKey: "secret-token"})
	if err != nil {
		t.Fatal(err)
	}
	if client.inactivityTimeout("openai/gpt-4o") != 5*time.Minute || defaultStreamInactivityTimeout != defaultStreamMaxDuration {
		t.Fatalf("default inactivity %v", client.inactivityTimeout("openai/gpt-4o"))
	}
}
