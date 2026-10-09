package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/domain"
)

// Stream emits validated text deltas and returns only a completed assistant message.
// A callback error terminates the stream immediately and is returned unchanged.
func (c *Client) Stream(ctx context.Context, req domain.CompletionRequest, onText func(domain.Text) error) (domain.CompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CompletionResult{}, err
	}
	if onText == nil {
		return domain.CompletionResult{}, errors.New("OpenRouter stream requires a text callback")
	}
	wire, err := mapRequest(req)
	if err != nil {
		return domain.CompletionResult{}, err
	}
	wire.Stream = true
	c.template(&wire)
	if c.endpoint != endpoint {
		// OpenAI-compatible servers such as vLLM report token usage in a
		// stream only when asked; OpenRouter always reports it.
		wire.StreamOptions = &streamOptionsDTO{IncludeUsage: true}
	}
	body, err := json.Marshal(c.body(wire))
	if err != nil {
		return domain.CompletionResult{}, errors.New("could not encode OpenRouter request")
	}
	streamCtx, cancelStream := context.WithCancelCause(ctx)
	defer cancelStream(nil)
	timeout := &StreamTimeoutError{provider: c.provider}
	inactivityTimer := time.AfterFunc(c.streamInactivityTimeout, func() { cancelStream(timeout) })
	defer inactivityTimer.Stop()
	maximumDurationTimeout := &StreamTimeoutError{maximumDuration: true, provider: c.provider}
	maximumDurationTimer := time.AfterFunc(c.streamMaxDuration, func() { cancelStream(maximumDurationTimeout) })
	defer maximumDurationTimer.Stop()
	streamError := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var streamTimeout *StreamTimeoutError
		if errors.As(context.Cause(streamCtx), &streamTimeout) {
			return streamTimeout
		}
		return nil
	}
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.CompletionResult{}, errors.New("could not create OpenRouter request")
	}
	c.authorize(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	response, err := c.http.Do(httpReq)
	if err != nil {
		if cause := streamError(); cause != nil {
			return domain.CompletionResult{}, cause
		}
		return domain.CompletionResult{}, &TransportError{Cause: err, Provider: c.provider}
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = response.Body.Close() }) }
	stop := context.AfterFunc(streamCtx, closeBody)
	defer func() { stop(); closeBody() }()
	if cause := streamError(); cause != nil {
		return domain.CompletionResult{}, cause
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.CompletionResult{}, c.providerError(response.StatusCode)
	}
	decoder := newSSEDecoder(response.Body)
	var accumulator streamAccumulator
	for {
		if cause := streamError(); cause != nil {
			return domain.CompletionResult{}, cause
		}
		data, err := decoder.Next()
		if cause := streamError(); cause != nil {
			return domain.CompletionResult{}, cause
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return domain.CompletionResult{}, errors.New(c.provider + " stream ended without [DONE]")
			}
			return domain.CompletionResult{}, &TransportError{Cause: err, Provider: c.provider}
		}
		inactivityTimer.Stop()
		if bytes.Equal(data, []byte("[DONE]")) {
			return accumulator.Result()
		}
		deltas, err := accumulator.Add(data)
		if err != nil {
			var provider *ProviderError
			if errors.As(err, &provider) && c.provider != providerName {
				provider.Provider = c.provider
			}
			return domain.CompletionResult{}, err
		}
		for _, text := range deltas {
			if err := ctx.Err(); err != nil {
				return domain.CompletionResult{}, err
			}
			if err := onText(text); err != nil {
				return domain.CompletionResult{}, err
			}
		}
		inactivityTimer.Reset(c.streamInactivityTimeout)
	}
}
