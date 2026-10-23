package anthropic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/adapters/provider"
	"github.com/underpass-ai/AXLR/domain"
)

const testTimeout = 40 * time.Millisecond

// blockingBody sends one event and then nothing until it is closed.
type blockingBody struct {
	sent   bool
	closed chan struct{}
}

func (b *blockingBody) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(p, sse(toolStream[0])), nil
	}
	<-b.closed
	return 0, io.EOF
}

func (b *blockingBody) Close() error {
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return nil
}

func assertTimedOut(t *testing.T, err error, maximum bool) {
	t.Helper()
	var timeout *provider.StreamTimeoutError
	if !errors.As(err, &timeout) || !errors.Is(err, context.DeadlineExceeded) || timeout.MaximumDuration() != maximum || timeout.Provider() != "model endpoint api.anthropic.com" {
		t.Fatalf("expected a stream timeout (maximum=%v), got %T: %v", maximum, err, err)
	}
	if strings.Contains(err.Error(), "sk-ant-secret") {
		t.Fatalf("timeout leaked the key: %v", err)
	}
}

func TestStreamTimesOutWaitingForHeaders(t *testing.T) {
	c, err := New(ClientConfig{APIKey: "sk-ant-secret", StreamInactivityTimeout: testTimeout, HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
	assertTimedOut(t, err, false)
}

func TestStreamTimesOutWhenSilentAndAtItsMaximumDuration(t *testing.T) {
	for _, maximum := range []bool{false, true} {
		body := &blockingBody{closed: make(chan struct{})}
		config := ClientConfig{StreamInactivityTimeout: testTimeout, StreamMaxDuration: time.Hour}
		if maximum {
			config = ClientConfig{StreamInactivityTimeout: time.Hour, StreamMaxDuration: testTimeout}
		}
		c, _ := testClient(t, config, 200, body, nil)
		done := make(chan error, 1)
		go func() {
			_, err := c.Stream(context.Background(), simpleRequest(), func(domain.Text) error { return nil })
			done <- err
		}()
		select {
		case err := <-done:
			assertTimedOut(t, err, maximum)
		case <-time.After(2 * time.Second):
			t.Fatal("stream remained blocked")
		}
	}
}

// Complete is bounded by the maximum duration too.
func TestCompleteTimesOutAtItsMaximumDuration(t *testing.T) {
	c, err := New(ClientConfig{APIKey: "sk-ant-secret", StreamMaxDuration: testTimeout, HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Complete(context.Background(), simpleRequest())
	assertTimedOut(t, err, true)
}
