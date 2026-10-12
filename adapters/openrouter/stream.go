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
// A request that fails once sent returns its RequestBytes, and its Provider
// when the stream named one, beside the error; never a message.
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
	var accumulator streamAccumulator
	// sent is what a request that failed once it was sent reports: the size
	// of its body and who served it, so the console can account for a
	// request the provider may bill although no usage arrived.
	sent := func() domain.CompletionResult {
		return domain.CompletionResult{RequestBytes: len(body), Provider: accumulator.provider}
	}
	response, err := c.http.Do(httpReq)
	if err != nil {
		if cause := streamError(); cause != nil {
			return sent(), cause
		}
		return sent(), &TransportError{Cause: err, Provider: c.provider}
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = response.Body.Close() }) }
	stop := context.AfterFunc(streamCtx, closeBody)
	defer func() { stop(); closeBody() }()
	if cause := streamError(); cause != nil {
		return sent(), cause
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sent(), c.providerError(response.StatusCode, errorResponseMessage(response.Body))
	}
	decoder := newSSEDecoder(response.Body)
	for {
		if cause := streamError(); cause != nil {
			return sent(), cause
		}
		data, err := decoder.Next()
		if cause := streamError(); cause != nil {
			return sent(), cause
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return sent(), errors.New(c.provider + " stream ended without [DONE]")
			}
			return sent(), &TransportError{Cause: err, Provider: c.provider}
		}
		inactivityTimer.Stop()
		if bytes.Equal(data, []byte("[DONE]")) {
			result, err := accumulator.Result()
			if err != nil {
				return sent(), err
			}
			// What the mapper added after the streamed text, the note on an
			// answer cut by the output limit, is streamed like the text.
			if streamed := len(accumulator.content); len(result.Message.Content) > streamed {
				if err := ctx.Err(); err != nil {
					return sent(), err
				}
				if err := onText(result.Message.Content[streamed:]); err != nil {
					return sent(), err
				}
			}
			result.RequestBytes = len(body)
			return result, nil
		}
		deltas, err := accumulator.Add(data)
		if err != nil {
			var provider *ProviderError
			if errors.As(err, &provider) {
				if c.provider != providerName {
					provider.Provider = c.provider
				}
				provider.withoutKey(c.apiKey)
			}
			return sent(), err
		}
		for _, text := range deltas {
			if err := ctx.Err(); err != nil {
				return sent(), err
			}
			if err := onText(text); err != nil {
				return sent(), err
			}
		}
		inactivityTimer.Reset(c.streamInactivityTimeout)
	}
}
